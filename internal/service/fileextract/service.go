package fileextract

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/fileextract/remote"
	"github.com/webapp/go-app/ai-agent/pkg/extract"
	"gorm.io/gorm"
)

// ProviderSupportsNativeFile 当前对话厂商是否支持 Files 上传并由模型直接读文件。
func ProviderSupportsNativeFile(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "qwen", "dashscope", "kimi", "moonshot":
		return true
	default:
		return false
	}
}

func NormalizeProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "dashscope":
		return "qwen"
	case "moonshot":
		return "kimi"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

// Service 文件缓存 / 云端上传 / 本机抽取。
type Service struct {
	db        *gorm.DB
	extractor *extract.Extractor
	root      string
	cfg       *config.Config
	locker    *DocLocker
}

func New(db *gorm.DB, extractor *extract.Extractor, attachmentsDir string, cfg *config.Config) *Service {
	dir := strings.TrimSpace(attachmentsDir)
	if dir == "" {
		dir = "attachments"
	}
	return &Service{db: db, extractor: extractor, root: dir, cfg: cfg, locker: NewDocLocker()}
}

// PrepareInput 为 analyze / 语料准备文件。
type PrepareInput struct {
	Filename string
	Data     []byte
	Force    bool
	Provider string // 当前对话厂商：决定云端上传还是本机抽取
	NeedText bool   // 语料入库等场景必须拿到正文（通义将同步拉正文）
}

// PrepareResult 准备结果。
type PrepareResult struct {
	Text           string
	RemoteFileID   string
	Mode           string // text | file_id
	ContentHash    string
	ExtractionID   uuid.UUID
	CacheHit       bool
	SourcePath     string
	TextPath       string
	ExtractBackend string
	ChatModelHint  string // 如 qwen-long
}

// Prepare 按厂商能力准备文件正文或 file_id。
// 读缓存走读锁；强刷/抽取走写锁（TryLock，抢不到 → ErrBusy）。
func (s *Service) Prepare(ctx context.Context, in PrepareInput) (*PrepareResult, error) {
	if s == nil {
		return nil, fmt.Errorf("fileextract 未配置")
	}
	if len(in.Data) == 0 {
		return nil, fmt.Errorf("empty file")
	}
	hash := ContentHash(in.Data)
	name := strings.TrimSpace(in.Filename)
	if name == "" {
		name = "upload"
	}
	provider := NormalizeProvider(in.Provider)
	if provider == "" && s.cfg != nil {
		provider = NormalizeProvider(s.cfg.LLM.DefaultProvider)
	}

	if !in.Force {
		if !s.locker.TryRLock(hash) {
			return nil, ErrBusy
		}
		if hit := s.loadTextCache(ctx, hash); hit != nil {
			s.locker.RUnlock(hash)
			return hit, nil
		}
		// 通义：已有 remote_file_id 且无正文时，可直接复用 file_id（不重传）
		if provider == "qwen" {
			if reuse := s.loadQwenFileID(ctx, hash); reuse != nil {
				s.locker.RUnlock(hash)
				return reuse, nil
			}
		}
		s.locker.RUnlock(hash)
	}

	// 强刷或缓存未命中：写锁互斥抽取；抢不到立即失败
	if !s.locker.TryLock(hash) {
		return nil, ErrBusy
	}
	defer s.locker.Unlock(hash)

	if !in.Force {
		if hit := s.loadTextCache(ctx, hash); hit != nil {
			return hit, nil
		}
		if provider == "qwen" {
			if reuse := s.loadQwenFileID(ctx, hash); reuse != nil {
				return reuse, nil
			}
		}
	}

	native := ProviderSupportsNativeFile(provider)
	switch {
	case native && provider == "qwen":
		return s.prepareQwen(ctx, hash, name, in.Data, in.NeedText)
	case native && provider == "kimi":
		return s.prepareKimi(ctx, hash, name, in.Data)
	default:
		return s.prepareLocal(ctx, hash, name, in.Data)
	}
}

// Resolve 兼容旧调用：等价于 Prepare（Provider 空则本机）。
func (s *Service) Resolve(ctx context.Context, in ResolveInput) (*Result, error) {
	pr, err := s.Prepare(ctx, PrepareInput{
		Filename: in.Filename, Data: in.Data, Force: in.Force, Provider: in.Backend,
	})
	if err != nil {
		return nil, err
	}
	return &Result{
		Text: pr.Text, ExtractionID: pr.ExtractionID, ContentHash: pr.ContentHash,
		CacheHit: pr.CacheHit, SourcePath: pr.SourcePath, TextPath: pr.TextPath,
		ExtractBackend: pr.ExtractBackend, RemoteFileID: pr.RemoteFileID,
	}, nil
}

// ResolveInput 兼容旧签名。
type ResolveInput struct {
	Filename string
	Data     []byte
	Force    bool
	Backend  string
}

type Result struct {
	Text           string
	ExtractionID   uuid.UUID
	ContentHash    string
	CacheHit       bool
	SourcePath     string
	TextPath       string
	ExtractBackend string
	RemoteFileID   string
}

func (s *Service) prepareLocal(ctx context.Context, hash, name string, data []byte) (*PrepareResult, error) {
	if s.extractor == nil {
		return nil, fmt.Errorf("本地 extractor 未配置")
	}
	text, err := s.extractor.FromBytes(name, data)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("本地抽取结果为空")
	}
	row, err := s.persistNew(ctx, persistArgs{
		Hash: hash, Name: name, Data: data, Text: text,
		Backend: "local", SoftDeleteOld: true,
	})
	if err != nil {
		return nil, err
	}
	return &PrepareResult{
		Text: text, Mode: "text", ContentHash: hash, ExtractionID: row.ID,
		SourcePath: row.SourcePath, TextPath: row.TextPath, ExtractBackend: "local",
	}, nil
}

func (s *Service) prepareKimi(ctx context.Context, hash, name string, data []byte) (*PrepareResult, error) {
	cli, err := s.filesClient("kimi")
	if err != nil {
		return nil, err
	}
	fo, err := cli.Upload(ctx, name, data, "file-extract")
	if err != nil {
		return nil, fmt.Errorf("kimi upload: %w", err)
	}
	_, _ = cli.WaitProcessed(ctx, fo.ID, 90*time.Second)
	text, err := cli.Content(ctx, fo.ID)
	if err != nil {
		return nil, fmt.Errorf("kimi content: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("kimi 未返回文件正文")
	}
	row, err := s.persistNew(ctx, persistArgs{
		Hash: hash, Name: name, Data: data, Text: text,
		Backend: "kimi", RemoteID: fo.ID, SoftDeleteOld: true,
	})
	if err != nil {
		return nil, err
	}
	return &PrepareResult{
		Text: text, RemoteFileID: fo.ID, Mode: "text", ContentHash: hash,
		ExtractionID: row.ID, SourcePath: row.SourcePath, TextPath: row.TextPath,
		ExtractBackend: "kimi",
	}, nil
}

func (s *Service) prepareQwen(ctx context.Context, hash, name string, data []byte, needText bool) (*PrepareResult, error) {
	cli, err := s.filesClient("qwen")
	if err != nil {
		return nil, err
	}
	fo, err := cli.Upload(ctx, name, data, "file-extract")
	if err != nil {
		return nil, fmt.Errorf("qwen upload: %w", err)
	}
	_, _ = cli.WaitProcessed(ctx, fo.ID, 90*time.Second)

	text := ""
	if needText {
		text, err = cli.Content(ctx, fo.ID)
		if err != nil || strings.TrimSpace(text) == "" {
			text, err = cli.ChatWithFileID(ctx, "qwen-long", fo.ID, "请原样输出该文件的全部文字内容，不要总结。")
			if err != nil {
				return nil, fmt.Errorf("qwen 获取正文失败: %w", err)
			}
		}
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("qwen 未返回文件正文")
		}
	}

	row, err := s.persistNew(ctx, persistArgs{
		Hash: hash, Name: name, Data: data, Text: text,
		Backend: "qwen", RemoteID: fo.ID, SoftDeleteOld: true,
	})
	if err != nil {
		return nil, err
	}

	if !needText {
		go s.asyncFillQwenText(fo.ID, row.ID, hash)
		return &PrepareResult{
			RemoteFileID: fo.ID, Mode: "file_id", ContentHash: hash,
			ExtractionID: row.ID, SourcePath: row.SourcePath, TextPath: row.TextPath,
			ExtractBackend: "qwen", ChatModelHint: "qwen-long",
		}, nil
	}
	return &PrepareResult{
		Text: text, RemoteFileID: fo.ID, Mode: "text", ContentHash: hash,
		ExtractionID: row.ID, SourcePath: row.SourcePath, TextPath: row.TextPath,
		ExtractBackend: "qwen", ChatModelHint: "qwen-long",
	}, nil
}

func (s *Service) asyncFillQwenText(remoteID string, rowID uuid.UUID, hash string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cli, err := s.filesClient("qwen")
	if err != nil {
		return
	}
	text, err := cli.Content(ctx, remoteID)
	if err != nil || strings.TrimSpace(text) == "" {
		text, err = cli.ChatWithFileID(ctx, "qwen-long", remoteID, "请原样输出该文件的全部文字内容，不要总结。")
		if err != nil || strings.TrimSpace(text) == "" {
			return
		}
	}
	_ = s.writeTextFileAndUpdate(ctx, rowID, hash, text)
}

func (s *Service) writeTextFileAndUpdate(ctx context.Context, rowID uuid.UUID, hash, text string) error {
	dayDir := time.Now().Format("2006/01/02")
	absDay := filepath.Join(s.root, filepath.FromSlash(dayDir))
	if err := os.MkdirAll(absDay, 0o755); err != nil {
		return err
	}
	textName := rowID.String() + ".txt"
	absText := filepath.Join(absDay, textName)
	relText := filepath.ToSlash(filepath.Join(s.root, dayDir, textName))
	if err := os.WriteFile(absText, []byte(text), 0o644); err != nil {
		return err
	}
	if s.db == nil {
		return nil
	}
	return s.db.WithContext(ctx).Model(&model.FileExtraction{}).
		Where("id = ?", rowID).
		Updates(map[string]any{
			"text_path":  relText,
			"text_chars": utf8.RuneCountInString(text),
			"updated_at": time.Now(),
		}).Error
}

type persistArgs struct {
	Hash, Name, Text, Backend, RemoteID string
	Data                                []byte
	SoftDeleteOld                       bool
}

func (s *Service) persistNew(ctx context.Context, a persistArgs) (*model.FileExtraction, error) {
	id := uuid.New()
	ext := strings.ToLower(filepath.Ext(a.Name))
	dayDir := time.Now().Format("2006/01/02")
	absDay := filepath.Join(s.root, filepath.FromSlash(dayDir))
	if err := os.MkdirAll(absDay, 0o755); err != nil {
		return nil, fmt.Errorf("创建附件目录失败: %w", err)
	}
	sourceName := id.String() + "_source" + ext
	absSource := filepath.Join(absDay, sourceName)
	relSource := filepath.ToSlash(filepath.Join(s.root, dayDir, sourceName))
	if err := os.WriteFile(absSource, a.Data, 0o644); err != nil {
		return nil, fmt.Errorf("保存原始文件失败: %w", err)
	}

	relText := ""
	textChars := 0
	if strings.TrimSpace(a.Text) != "" {
		textName := id.String() + ".txt"
		absText := filepath.Join(absDay, textName)
		relText = filepath.ToSlash(filepath.Join(s.root, dayDir, textName))
		if err := os.WriteFile(absText, []byte(a.Text), 0o644); err != nil {
			return nil, fmt.Errorf("保存抽取文本失败: %w", err)
		}
		textChars = utf8.RuneCountInString(a.Text)
	}

	row := &model.FileExtraction{
		ID: id, ContentHash: a.Hash, OriginalName: a.Name, Ext: ext,
		SizeBytes: int64(len(a.Data)), SourcePath: relSource, TextPath: relText,
		TextChars: textChars, ExtractBackend: a.Backend, RemoteFileID: a.RemoteID,
		Status: "ready", IsDeleted: 0,
	}
	if s.db == nil {
		// 最小化部署：只落盘附件/文本，不写库、不缓存命中
		return row, nil
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, fmt.Errorf("写入 file_extractions 失败: %w", err)
	}
	// 新记录成功后再软删旧行
	if a.SoftDeleteOld {
		_ = s.softDeleteActiveExcept(ctx, a.Hash, id)
	}
	return row, nil
}

func (s *Service) softDeleteActiveExcept(ctx context.Context, hash string, keep uuid.UUID) error {
	if s.db == nil {
		return nil
	}
	return s.db.WithContext(ctx).Model(&model.FileExtraction{}).
		Where("content_hash = ? AND is_deleted = 0 AND id <> ?", hash, keep).
		Updates(map[string]any{"is_deleted": 1, "updated_at": time.Now()}).Error
}

func (s *Service) loadTextCache(ctx context.Context, hash string) *PrepareResult {
	if s.db == nil {
		return nil
	}
	var row model.FileExtraction
	err := s.db.WithContext(ctx).
		Where("content_hash = ? AND is_deleted = 0 AND status = ? AND text_path <> ''", hash, "ready").
		Order("created_at desc").First(&row).Error
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(filepath.FromSlash(row.TextPath))
	if err != nil || strings.TrimSpace(string(b)) == "" {
		return nil
	}
	return &PrepareResult{
		Text: string(b), Mode: "text", ContentHash: hash, ExtractionID: row.ID,
		CacheHit: true, SourcePath: row.SourcePath, TextPath: row.TextPath,
		ExtractBackend: row.ExtractBackend, RemoteFileID: row.RemoteFileID,
	}
}

func (s *Service) loadQwenFileID(ctx context.Context, hash string) *PrepareResult {
	if s.db == nil {
		return nil
	}
	var row model.FileExtraction
	err := s.db.WithContext(ctx).
		Where("content_hash = ? AND is_deleted = 0 AND status = ? AND extract_backend = ? AND remote_file_id <> ''",
			hash, "ready", "qwen").
		Order("created_at desc").First(&row).Error
	if err != nil {
		return nil
	}
	return &PrepareResult{
		RemoteFileID: row.RemoteFileID, Mode: "file_id", ContentHash: hash,
		ExtractionID: row.ID, CacheHit: true, SourcePath: row.SourcePath,
		TextPath: row.TextPath, ExtractBackend: "qwen", ChatModelHint: "qwen-long",
	}
}

func (s *Service) filesClient(provider string) (*remote.FilesClient, error) {
	if s.cfg == nil {
		return nil, fmt.Errorf("config 未配置")
	}
	_, pc, err := s.cfg.ResolveLLM(provider)
	if err != nil {
		return nil, err
	}
	timeout := 180 * time.Second
	if s.cfg.Extract.TimeoutSeconds > 0 {
		timeout = time.Duration(s.cfg.Extract.TimeoutSeconds) * time.Second
	} else if s.cfg.LLM.TimeoutSeconds > 0 {
		timeout = time.Duration(s.cfg.LLM.TimeoutSeconds) * time.Second
	}
	return remote.NewFilesClient(pc.BaseURL, pc.APIKey, timeout), nil
}

func ContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
