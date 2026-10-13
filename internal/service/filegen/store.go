package filegen

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound 文件不存在、编号无效或已过期。
var ErrNotFound = errors.New("export not found")

// Store 把生成的 Excel 落到本地目录，并用不可猜测的编号提供下载。
type Store struct {
	dir        string
	publicBase string
	ttl        time.Duration
}

type meta struct {
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
}

// Open 创建或复用导出目录。ttl <= 0 表示文件不过期。publicBase 为空时下载地址只有站内路径。
func Open(dir, publicBase string, ttl time.Duration) (*Store, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "exports"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{
		dir:        dir,
		publicBase: strings.TrimRight(strings.TrimSpace(publicBase), "/"),
		ttl:        ttl,
	}, nil
}

// File 一次导出的下载信息。
type File struct {
	ID   string
	Name string
	URL  string
}

// SaveXLSX 写入工作簿。columns 为表头，rows 与之对齐。
func (s *Store) SaveXLSX(title string, columns []string, rows [][]string) (File, error) {
	if s == nil {
		return File{}, errors.New("文件生成服务未就绪")
	}
	body, name, err := buildXLSX(title, columns, rows)
	if err != nil {
		return File{}, err
	}
	id := uuid.NewString()
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return File{}, err
	}
	defer root.Close()
	if err := writeFile(root, id+".xlsx", body); err != nil {
		return File{}, err
	}
	raw, err := json.Marshal(meta{Name: name, Created: time.Now().UTC()})
	if err != nil {
		_ = root.Remove(id + ".xlsx")
		return File{}, err
	}
	if err := writeFile(root, id+".json", raw); err != nil {
		_ = root.Remove(id + ".xlsx")
		return File{}, err
	}
	s.purgeExpired(root)
	return File{ID: id, Name: name, URL: s.url(id)}, nil
}

// Read 按编号读取未过期文件。编号必须是生成时的 UUID。
func (s *Store) Read(id string) (name string, body []byte, err error) {
	if s == nil {
		return "", nil, ErrNotFound
	}
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return "", nil, ErrNotFound
	}
	id = parsed.String()
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return "", nil, err
	}
	defer root.Close()
	raw, err := readFile(root, id+".json")
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, ErrNotFound
		}
		return "", nil, err
	}
	var info meta
	if err := json.Unmarshal(raw, &info); err != nil || strings.TrimSpace(info.Name) == "" {
		return "", nil, ErrNotFound
	}
	if s.ttl > 0 && time.Since(info.Created) > s.ttl {
		_ = root.Remove(id + ".xlsx")
		_ = root.Remove(id + ".json")
		return "", nil, ErrNotFound
	}
	body, err = readFile(root, id+".xlsx")
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, ErrNotFound
		}
		return "", nil, err
	}
	return info.Name, body, nil
}

func (s *Store) url(id string) string {
	path := "/api/v1/exports/" + id
	if s.publicBase == "" {
		return path
	}
	return s.publicBase + path
}

func writeFile(root *os.Root, name string, body []byte) error {
	f, err := root.Create(name)
	if err != nil {
		return err
	}
	_, werr := f.Write(body)
	cerr := f.Close()
	if werr != nil {
		_ = root.Remove(name)
		return werr
	}
	return cerr
}

func readFile(root *os.Root, name string) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (s *Store) purgeExpired(root *os.Root) {
	if s == nil || s.ttl <= 0 || root == nil {
		return
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, ent := range entries {
		name := ent.Name()
		if !strings.HasSuffix(name, ".json") || ent.IsDir() {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if _, err := uuid.Parse(id); err != nil {
			continue
		}
		raw, err := readFile(root, name)
		if err != nil {
			continue
		}
		var info meta
		if json.Unmarshal(raw, &info) != nil {
			continue
		}
		if time.Since(info.Created) > s.ttl {
			_ = root.Remove(id + ".xlsx")
			_ = root.Remove(name)
		}
	}
}
