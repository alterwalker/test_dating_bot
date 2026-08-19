package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/alterwalker/test_dating_bot/internal/domain"
	"github.com/google/uuid"
)

func matchCardKeyboard(candidateID uuid.UUID) string {
	id := candidateID.String()
	return fmt.Sprintf(`{"inline_keyboard":[[{"text":"📋 Показать профиль","callback_data":"candidate:%s"},{"text":"💬 Начать общение","callback_data":"icebreaker:%s"}],[{"text":"🚫 Не показывать","callback_data":"hide:ask:%s"}]]}`, id, id, id)
}

func hideConfirmKeyboard(candidateID uuid.UUID) string {
	id := candidateID.String()
	return fmt.Sprintf(`{"inline_keyboard":[[{"text":"✅ Да, скрыть","callback_data":"hide:yes:%s"},{"text":"❌ Отмена","callback_data":"hide:cancel"}]]}`, id)
}

func handleHideCallback(ctx context.Context, client *apiClient, token string, chatID int64, userID, data string) error {
	parts := strings.Split(data, ":")
	if len(parts) < 2 || parts[0] != "hide" {
		return nil
	}
	switch parts[1] {
	case "ask":
		if len(parts) < 3 {
			return nil
		}
		candidateID, ok := parseCandidateID(parts[2])
		if !ok {
			return nil
		}
		return sendMessageWithKeyboard(ctx, token, chatID,
			"Скрыть эту анкету?\n\nОна больше не будет появляться в ваших Matches.",
			hideConfirmKeyboard(candidateID))
	case "yes":
		if len(parts) < 3 {
			return nil
		}
		candidateID, ok := parseCandidateID(parts[2])
		if !ok {
			return nil
		}
		if err := client.postJSON(ctx, fmt.Sprintf("/users/%s/matches/%s/hide", userID, candidateID.String()), nil, nil); err != nil {
			return sendWithMainMenu(ctx, token, chatID, "Не удалось скрыть анкету: "+err.Error())
		}
		return sendWithMainMenu(ctx, token, chatID, "Анкета скрыта. Нажмите «🔍 Matches», чтобы обновить список.")
	case "cancel":
		return sendWithMainMenu(ctx, token, chatID, "Скрытие отменено.")
	default:
		return nil
	}
}

func parseCandidateID(s string) (uuid.UUID, bool) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func showCandidateProfile(ctx context.Context, client *apiClient, token string, chatID int64, userID, candidateID string) error {
	var prof domain.CandidateProfile
	if err := client.getJSON(ctx, "/users/"+userID+"/matches/"+candidateID, &prof); err != nil {
		if strings.Contains(err.Error(), "profile not confirmed") {
			return sendWithMainMenu(ctx, token, chatID, "Сначала заполните анкету — нажмите «👤 Профиль».")
		}
		return sendWithMainMenu(ctx, token, chatID, "Не удалось загрузить профиль: "+err.Error())
	}

	text := formatCandidateProfile(prof)
	keyboard := matchCardKeyboard(prof.CandidateID)
	return sendMessageWithKeyboard(ctx, token, chatID, text, keyboard)
}

func formatCandidateProfile(p domain.CandidateProfile) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("👤 %s, %d · %s\n", p.Name, p.Age, p.City))

	if label := genderLabels[p.Gender]; label != "" {
		b.WriteString("⚧ Пол: " + label + "\n")
	}
	if intent := intentLabels[p.RelationshipIntent]; intent != "" {
		b.WriteString("🎯 Цель: " + intent + "\n")
	}
	if p.IsFictional {
		b.WriteString("🤖 Demo-анкета\n")
	}

	if p.Summary != "" {
		b.WriteString("\n📝 " + p.Summary + "\n")
	}
	if len(p.Interests) > 0 {
		b.WriteString("\n🏷 Интересы: " + strings.Join(p.Interests, ", ") + "\n")
	}
	if len(p.Values) > 0 {
		b.WriteString("💎 Ценности: " + strings.Join(p.Values, ", ") + "\n")
	}
	if len(p.SharedInterests) > 0 {
		b.WriteString("\n🤝 Общие интересы: " + strings.Join(p.SharedInterests, ", ") + "\n")
	}
	if len(p.SharedValues) > 0 {
		b.WriteString("🤝 Общие ценности: " + strings.Join(p.SharedValues, ", ") + "\n")
	}

	if p.IdealEvening != "" {
		b.WriteString("\n🌙 Идеальный вечер:\n" + truncateText(p.IdealEvening, 300) + "\n")
	}
	if p.RelationshipValues != "" {
		b.WriteString("\n💬 Важно в отношениях:\n" + truncateText(p.RelationshipValues, 300) + "\n")
	}
	if p.InterestsText != "" {
		b.WriteString("\n🎨 Интересы помимо работы:\n" + truncateText(p.InterestsText, 300) + "\n")
	}

	if p.CommunicationStyle != "" {
		b.WriteString("\n🗣 Стиль общения: " + p.CommunicationStyle)
	}

	return strings.TrimSpace(b.String())
}

func parseCandidateCallback(data string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimPrefix(data, "candidate:"))
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}
