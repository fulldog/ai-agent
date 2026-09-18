package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dingtalkcard "github.com/alibabacloud-go/dingtalk/card_1_0"
	dingtalkoauth2 "github.com/alibabacloud-go/dingtalk/oauth2_1_0"
	dingtalkrobot "github.com/alibabacloud-go/dingtalk/robot_1_0"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/google/uuid"
)

const cardContentKey = "content"

type openAPI struct {
	clientID     string
	clientSecret string
	robotCode    string
	http         *http.Client
	oauth        *dingtalkoauth2.Client
	card         *dingtalkcard.Client
	robot        *dingtalkrobot.Client
	initErr      error

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

func dingClientConfig() *openapi.Config {
	return &openapi.Config{
		Protocol: tea.String("https"),
		RegionId: tea.String("central"),
	}
}

func newOpenAPI(clientID, clientSecret string) *openAPI {
	a := &openAPI{
		clientID:     clientID,
		clientSecret: clientSecret,
		robotCode:    clientID,
		http:         &http.Client{Timeout: 20 * time.Second},
	}
	var errs []error
	oauth, err := dingtalkoauth2.NewClient(dingClientConfig())
	if err != nil {
		errs = append(errs, fmt.Errorf("oauth2 client: %w", err))
	}
	card, err := dingtalkcard.NewClient(dingClientConfig())
	if err != nil {
		errs = append(errs, fmt.Errorf("card client: %w", err))
	}
	robot, err := dingtalkrobot.NewClient(dingClientConfig())
	if err != nil {
		errs = append(errs, fmt.Errorf("robot client: %w", err))
	}
	a.oauth = oauth
	a.card = card
	a.robot = robot
	a.initErr = errors.Join(errs...)
	return a
}

func (a *openAPI) accessToken(ctx context.Context) (string, error) {
	if a == nil {
		return "", errors.New("dingtalk openapi nil")
	}
	if a.initErr != nil {
		return "", a.initErr
	}
	if a.oauth == nil {
		return "", errors.New("dingtalk oauth2 client nil")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	a.mu.Lock()
	if a.token != "" && time.Now().Before(a.tokenExpiry) {
		tok := a.token
		a.mu.Unlock()
		return tok, nil
	}
	a.mu.Unlock()

	resp, err := a.oauth.GetAccessToken(&dingtalkoauth2.GetAccessTokenRequest{
		AppKey:    tea.String(a.clientID),
		AppSecret: tea.String(a.clientSecret),
	})
	if err != nil {
		return "", fmt.Errorf("dingtalk access token: %w", err)
	}
	if resp == nil || resp.Body == nil || tea.StringValue(resp.Body.AccessToken) == "" {
		return "", errors.New("dingtalk access token: empty response")
	}
	ttl := time.Duration(tea.Int64Value(resp.Body.ExpireIn)) * time.Second
	if ttl <= 0 {
		ttl = 90 * time.Minute
	}
	tok := tea.StringValue(resp.Body.AccessToken)
	a.mu.Lock()
	a.token = tok
	a.tokenExpiry = time.Now().Add(ttl - 2*time.Minute)
	a.mu.Unlock()
	return tok, nil
}

func (a *openAPI) createCard(ctx context.Context, templateID, conversationID, conversationType, staffID, seed string) (string, error) {
	if a.card == nil {
		return "", errors.New("dingtalk card client nil")
	}
	tok, err := a.accessToken(ctx)
	if err != nil {
		return "", err
	}
	outTrackID := uuid.NewString()
	req := &dingtalkcard.CreateAndDeliverRequest{
		CardTemplateId: tea.String(templateID),
		OutTrackId:     tea.String(outTrackID),
		CardData: &dingtalkcard.CreateAndDeliverRequestCardData{
			CardParamMap: map[string]*string{
				cardContentKey: tea.String(seed),
			},
		},
	}
	if isGroup(conversationType) {
		req.OpenSpaceId = tea.String("dtv1.card//IM_GROUP." + conversationID)
		req.ImGroupOpenSpaceModel = &dingtalkcard.CreateAndDeliverRequestImGroupOpenSpaceModel{
			SupportForward: tea.Bool(true),
		}
		req.ImGroupOpenDeliverModel = &dingtalkcard.CreateAndDeliverRequestImGroupOpenDeliverModel{
			RobotCode: tea.String(a.robotCode),
		}
	} else {
		uid := staffID
		if uid == "" {
			uid = conversationID
		}
		req.OpenSpaceId = tea.String("dtv1.card//IM_ROBOT." + uid)
		req.ImRobotOpenDeliverModel = &dingtalkcard.CreateAndDeliverRequestImRobotOpenDeliverModel{
			SpaceType: tea.String("IM_ROBOT"),
			RobotCode: tea.String(a.robotCode),
		}
	}
	headers := &dingtalkcard.CreateAndDeliverHeaders{}
	headers.XAcsDingtalkAccessToken = tea.String(tok)
	_, err = a.card.CreateAndDeliverWithOptions(req, headers, &util.RuntimeOptions{})
	if err != nil {
		return "", fmt.Errorf("create and deliver card: %w", err)
	}
	return outTrackID, nil
}

func (a *openAPI) streamCard(ctx context.Context, outTrackID, content string, finalize, isError bool) error {
	if a.card == nil {
		return errors.New("dingtalk card client nil")
	}
	tok, err := a.accessToken(ctx)
	if err != nil {
		return err
	}
	headers := &dingtalkcard.StreamingUpdateHeaders{}
	headers.XAcsDingtalkAccessToken = tea.String(tok)
	_, err = a.card.StreamingUpdateWithOptions(&dingtalkcard.StreamingUpdateRequest{
		OutTrackId: tea.String(outTrackID),
		Guid:       tea.String(uuid.NewString()),
		Key:        tea.String(cardContentKey),
		Content:    tea.String(content),
		IsFull:     tea.Bool(true),
		IsFinalize: tea.Bool(finalize),
		IsError:    tea.Bool(isError),
	}, headers, &util.RuntimeOptions{})
	if err != nil {
		return fmt.Errorf("stream card: %w", err)
	}
	return nil
}

func (a *openAPI) sendGroupMarkdown(ctx context.Context, openConversationID, title, text string) error {
	if a.robot == nil {
		return errors.New("dingtalk robot client nil")
	}
	tok, err := a.accessToken(ctx)
	if err != nil {
		return err
	}
	param, err := json.Marshal(map[string]string{"title": title, "text": text})
	if err != nil {
		return fmt.Errorf("marshal group markdown: %w", err)
	}
	headers := &dingtalkrobot.OrgGroupSendHeaders{}
	headers.XAcsDingtalkAccessToken = tea.String(tok)
	_, err = a.robot.OrgGroupSendWithOptions(&dingtalkrobot.OrgGroupSendRequest{
		RobotCode:          tea.String(a.robotCode),
		OpenConversationId: tea.String(openConversationID),
		MsgKey:             tea.String("sampleMarkdown"),
		MsgParam:           tea.String(string(param)),
	}, headers, &util.RuntimeOptions{})
	if err != nil {
		return fmt.Errorf("org group send: %w", err)
	}
	return nil
}

func replyWebhook(ctx context.Context, webhook, title, text string) error {
	if strings.TrimSpace(webhook) == "" {
		return errors.New("empty session webhook")
	}
	body, err := json.Marshal(map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  text,
		},
	})
	if err != nil {
		return fmt.Errorf("marshal session webhook: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("session webhook: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read session webhook: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("session webhook: status=%d body=%s", resp.StatusCode, string(raw))
	}
	return nil
}
