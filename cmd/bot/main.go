package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alterwalker/test_dating_bot/internal/config"
	"github.com/alterwalker/test_dating_bot/internal/domain"
)

type apiClient struct {
	base string
	http *http.Client
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if cfg.BotToken == "" {
		log.Fatal("BOT_TOKEN is required")
	}

	client := &apiClient{base: strings.TrimRight(cfg.APIBaseURL, "/"), http: &http.Client{Timeout: 30 * time.Second}}
	pollClient := &http.Client{Timeout: 35 * time.Second}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	offset := int64(0)
	states := newStateStore()

	log.Printf("bot started, api=%s", cfg.APIBaseURL)

	for {
		if err := ctx.Err(); err != nil {
			log.Printf("bot stopped")
			return
		}

		updates, err := getUpdates(ctx, pollClient, cfg.BotToken, offset)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
				log.Printf("bot stopped")
				return
			}
			log.Printf("updates: %v", err)
			if !sleepCtx(ctx, 2*time.Second) {
				log.Printf("bot stopped")
				return
			}
			continue
		}
		for _, upd := range updates {
			offset = upd.UpdateID + 1
			if err := handleUpdate(ctx, client, states, cfg.BotToken, upd); err != nil {
				log.Printf("handle update: %v", err)
			}
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

type tgUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64  `json:"message_id"`
		Text      string `json:"text"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From *struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"from"`
	} `json:"message"`
	CallbackQuery *struct {
		ID   string `json:"id"`
		Data string `json:"data"`
		From struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Message struct {
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

func getUpdates(ctx context.Context, httpClient *http.Client, token string, offset int64) ([]tgUpdate, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?timeout=25&offset=%d", token, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		OK     bool       `json:"ok"`
		Result []tgUpdate `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Result, nil
}

func handleUpdate(ctx context.Context, client *apiClient, states *stateStore, token string, upd tgUpdate) error {
	if upd.CallbackQuery != nil {
		return handleCallback(ctx, client, states, token, upd.CallbackQuery.ID, upd.CallbackQuery.Data, upd.CallbackQuery.Message.Chat.ID, upd.CallbackQuery.From.ID)
	}
	if upd.Message == nil || upd.Message.From == nil {
		return nil
	}
	chatID := upd.Message.Chat.ID
	text := normalizeMenuText(strings.TrimSpace(upd.Message.Text))
	userID, err := client.ensureUser(ctx, upd.Message.From.ID, upd.Message.From.Username)
	if err != nil {
		return sendWithMainMenu(ctx, token, chatID, "Ошибка регистрации: "+err.Error())
	}

	if isMenuAction(text) {
		states.clear(chatID)
	}

	switch {
	case text == "/start" || text == "/cancel":
		return sendWithMainMenu(ctx, token, chatID, "Привет! Выберите действие кнопкой ниже или командой.")
	case text == "/profile":
		return startProfile(ctx, client, states, token, chatID, userID)
	case text == "/matches":
		return showMatches(ctx, client, states, token, chatID, userID)
	case text == "/admin":
		return showAdminStats(ctx, client, token, chatID)
	default:
		return handleProfileStep(ctx, client, states, token, chatID, userID, text)
	}
}

func handleCallback(ctx context.Context, client *apiClient, states *stateStore, token, callbackID, data string, chatID, telegramID int64) error {
	_ = answerCallback(ctx, token, callbackID)
	userID, err := client.ensureUser(ctx, telegramID, "")
	if err != nil {
		return err
	}
	if strings.HasPrefix(data, "profile:") {
		return handleProfileCallback(ctx, client, states, token, chatID, userID, data)
	}
	if strings.HasPrefix(data, "candidate:") {
		candidateID, ok := parseCandidateCallback(data)
		if !ok {
			return nil
		}
		return showCandidateProfile(ctx, client, token, chatID, userID, candidateID.String())
	}
	if strings.HasPrefix(data, "icebreaker:") {
		candidateID := strings.TrimPrefix(data, "icebreaker:")
		var result domain.IcebreakerResult
		if err := client.postJSON(ctx, fmt.Sprintf("/users/%s/matches/%s/icebreaker", userID, candidateID), nil, &result); err != nil {
			return sendWithMainMenu(ctx, token, chatID, "Не удалось сгенерировать icebreaker: "+err.Error())
		}
		var b strings.Builder
		name := result.CandidateName
		if name == "" {
			name = "кандидат"
		}
		b.WriteString("💬 Беседа с " + name + "\n\n")
		b.WriteString("💡 Темы для разговора:\n")
		for i, t := range result.ConversationTopics {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, t))
		}
		b.WriteString("\n✉️ Можно начать так:\n«" + result.OpenerMessage + "»")
		if id, ok := parseCandidateCallback("candidate:" + candidateID); ok {
			return sendMessageWithKeyboard(ctx, token, chatID, b.String(), matchCardKeyboard(id))
		}
		return sendWithMainMenu(ctx, token, chatID, b.String())
	}
	return nil
}

func showMatches(ctx context.Context, client *apiClient, states *stateStore, token string, chatID int64, userID string) error {
	_ = states
	var resp struct {
		Matches []domain.Match `json:"matches"`
		Total   int            `json:"total_candidates_after_filters"`
	}
	if err := client.getJSON(ctx, "/users/"+userID+"/matches", &resp); err != nil {
		if strings.Contains(err.Error(), "profile not confirmed") {
			return sendWithMainMenu(ctx, token, chatID, "Сначала заполните анкету — нажмите «👤 Профиль».")
		}
		return sendWithMainMenu(ctx, token, chatID, "Ошибка: "+err.Error())
	}
	if len(resp.Matches) == 0 {
		prof, _ := loadRawProfile(ctx, client, userID)
		var b strings.Builder
		b.WriteString("Пока никого не нашли")
		if resp.Total == 0 {
			b.WriteString(" (0 кандидатов после фильтров).\n\n")
			b.WriteString("Проверьте:\n")
			b.WriteString("• город — в seed в основном Москва и Санкт-Петербург\n")
			b.WriteString("• кого ищете и возрастной диапазон\n")
			if prof.City != "" {
				b.WriteString(fmt.Sprintf("\nВаша анкета: %s, ищете %s", prof.City, formatSeeking(prof.Seeking)))
				if prof.AgeMin != nil && prof.AgeMax != nil {
					b.WriteString(fmt.Sprintf(", возраст %d–%d", *prof.AgeMin, *prof.AgeMax))
				}
			}
			b.WriteString("\n\nОбновить анкету: «👤 Профиль»")
		} else {
			b.WriteString(".")
		}
		return sendWithMainMenu(ctx, token, chatID, b.String())
	}
	for _, m := range resp.Matches {
		text := fmt.Sprintf("👤 %s, %d · ⭐ %.0f%%\n%s", m.CandidateName, m.CandidateAge, m.Score*100, m.Explanation)
		if err := sendMessageWithKeyboard(ctx, token, chatID, text, matchCardKeyboard(m.CandidateID)); err != nil {
			return err
		}
	}
	return nil
}

func (c *apiClient) ensureUser(ctx context.Context, telegramID int64, username string) (string, error) {
	var user domain.User
	body := map[string]any{"telegram_id": telegramID}
	if username != "" {
		body["username"] = username
	}
	if err := c.postJSON(ctx, "/users", body, &user); err != nil {
		return "", err
	}
	return user.ID.String(), nil
}

func (c *apiClient) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *apiClient) postJSON(ctx context.Context, path string, body any, out any) error {
	return c.doJSON(ctx, http.MethodPost, path, body, out)
}

func (c *apiClient) putJSON(ctx context.Context, path string, body any, out any) error {
	return c.doJSON(ctx, http.MethodPut, path, body, out)
}

func (c *apiClient) deleteJSON(ctx context.Context, path string) error {
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil)
}

func (c *apiClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, string(b))
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func sendMessage(ctx context.Context, token string, chatID int64, text string) error {
	return sendMessageWithKeyboard(ctx, token, chatID, text, "")
}

func sendMessageWithKeyboard(ctx context.Context, token string, chatID int64, text, replyMarkup string) error {
	payload := map[string]any{"chat_id": chatID, "text": text}
	if replyMarkup != "" {
		payload["reply_markup"] = json.RawMessage(replyMarkup)
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/sendMessage", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram: %s", string(body))
	}
	return nil
}

func answerCallback(ctx context.Context, token, callbackID string) error {
	payload := map[string]any{"callback_query_id": callbackID}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/answerCallbackQuery", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
