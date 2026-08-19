package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/alterwalker/test_dating_bot/internal/domain"
)

const (
	stepName      = "name"
	stepAge       = "age"
	stepCity      = "city"
	stepGender    = "gender"
	stepSeeking   = "seeking"
	stepAgeRange  = "age_range"
	stepIntent    = "intent"
	stepEvening   = "evening"
	stepValues    = "values"
	stepInterests = "interests"
)

var intentLabels = map[string]string{
	"serious":    "Серьёзные отношения",
	"casual":     "Лёгкое общение",
	"friendship": "Дружба",
	"unsure":     "Пока не знаю",
}

var profileStepOrder = []string{
	stepName, stepAge, stepCity, stepGender, stepSeeking, stepAgeRange,
	stepIntent, stepEvening, stepValues, stepInterests,
}

var genderLabels = map[string]string{
	"male":   "Мужской",
	"female": "Женский",
}

func startProfile(ctx context.Context, client *apiClient, states *stateStore, token string, chatID int64, userID string) error {
	prof, err := loadProfile(ctx, client, userID)
	if err != nil {
		return sendWithMainMenu(ctx, token, chatID, "Ошибка загрузки профиля: "+err.Error())
	}

	raw := prof.Raw
	if raw.ValidForEnrich() || prof.Status == domain.ProfileConfirmed || prof.Status == domain.ProfileReady {
		states.set(chatID, stepName, "review")
		if err := sendMessageWithKeyboard(ctx, token, chatID, profileOverview(raw, prof.Enriched), profileDeleteKeyboard()); err != nil {
			return err
		}
		return promptReviewStep(ctx, token, chatID, stepName, raw)
	}

	first := firstMissingStep(raw)
	states.set(chatID, first, "create")
	if profileHasData(prof) {
		_ = sendMessageWithKeyboard(ctx, token, chatID, "Анкета заполнена частично — можно продолжить или удалить и начать заново.", profileDeleteKeyboard())
	}
	return promptCreateStep(ctx, token, chatID, first, raw)
}

func profileDeleteKeyboard() string {
	return `{"inline_keyboard":[[{"text":"🗑 Удалить анкету","callback_data":"profile:delete:ask"}]]}`
}

func profileDeleteConfirmKeyboard() string {
	return `{"inline_keyboard":[[{"text":"✅ Да, удалить","callback_data":"profile:delete:yes"},{"text":"❌ Отмена","callback_data":"profile:delete:cancel"}]]}`
}

func profileHasData(prof domain.Profile) bool {
	if prof.Status != domain.ProfileDraft {
		return true
	}
	raw := prof.Raw
	return raw.Name != "" || raw.Age > 0 || raw.City != "" || raw.Gender != "" ||
		len(raw.Seeking) > 0 || raw.RelationshipIntent != "" ||
		raw.PromptIdealEvening != "" || raw.PromptRelationshipValues != "" || raw.PromptOccupation != ""
}

func deleteProfile(ctx context.Context, client *apiClient, states *stateStore, token string, chatID int64, userID string) error {
	if err := client.deleteJSON(ctx, "/users/"+userID+"/profile"); err != nil {
		return sendWithMainMenu(ctx, token, chatID, "Не удалось удалить анкету: "+err.Error())
	}
	states.clear(chatID)
	return sendWithMainMenu(ctx, token, chatID, "Анкета удалена. Нажмите «👤 Профиль», чтобы заполнить заново.")
}

func profileOverview(raw domain.RawProfile, enriched *domain.EnrichedProfile) string {
	var b strings.Builder
	b.WriteString("Ваш профиль заполнен.\n\n")
	b.WriteString("Пройдём по полям — каждое можно оставить или изменить.\n\n")
	b.WriteString(formatStepValue(raw, stepName) + "\n")
	b.WriteString(formatStepValue(raw, stepAge) + "\n")
	b.WriteString(formatStepValue(raw, stepCity) + "\n")
	b.WriteString(formatStepValue(raw, stepGender) + "\n")
	b.WriteString(formatStepValue(raw, stepSeeking) + "\n")
	b.WriteString(formatStepValue(raw, stepAgeRange) + "\n")
	b.WriteString(formatStepValue(raw, stepIntent) + "\n")
	if enriched != nil && enriched.Summary != "" {
		b.WriteString("\n📝 " + enriched.Summary)
	}
	return b.String()
}

func firstMissingStep(raw domain.RawProfile) string {
	for _, step := range profileStepOrder {
		if !stepFilled(raw, step) {
			return step
		}
	}
	return stepName
}

func stepFilled(raw domain.RawProfile, step string) bool {
	switch step {
	case stepName:
		return raw.Name != ""
	case stepAge:
		return raw.Age > 0
	case stepCity:
		return raw.City != ""
	case stepGender:
		return raw.Gender != ""
	case stepSeeking:
		return len(raw.Seeking) > 0
	case stepAgeRange:
		return true // optional field
	case stepIntent:
		return raw.RelationshipIntent != ""
	case stepEvening:
		return len(raw.PromptIdealEvening) >= 10
	case stepValues:
		return len(raw.PromptRelationshipValues) >= 10
	case stepInterests:
		return len(raw.PromptOccupation) >= 10
	default:
		return false
	}
}

func stepTitle(step string) string {
	switch step {
	case stepName:
		return "Имя"
	case stepAge:
		return "Возраст"
	case stepCity:
		return "Город"
	case stepGender:
		return "Пол"
	case stepSeeking:
		return "Кого ищете"
	case stepAgeRange:
		return "Возрастной диапазон"
	case stepIntent:
		return "Цель знакомства"
	case stepEvening:
		return "Идеальный вечер"
	case stepValues:
		return "Ценности в отношениях"
	case stepInterests:
		return "Интересы помимо работы"
	default:
		return step
	}
}

func formatStepValue(raw domain.RawProfile, step string) string {
	switch step {
	case stepName:
		return "👤 Имя: " + raw.Name
	case stepAge:
		if raw.Age > 0 {
			return fmt.Sprintf("🎂 Возраст: %d", raw.Age)
		}
		return "🎂 Возраст: не указан"
	case stepCity:
		return "📍 Город: " + raw.City
	case stepGender:
		label := genderLabels[raw.Gender]
		if label == "" {
			label = raw.Gender
		}
		return "⚧ Пол: " + label
	case stepSeeking:
		if len(raw.Seeking) == 0 {
			return "👥 Кого ищете: не указано"
		}
		return "👥 Кого ищете: " + formatSeeking(raw.Seeking)
	case stepAgeRange:
		if raw.AgeMin != nil && raw.AgeMax != nil {
			return fmt.Sprintf("📅 Возраст партнёра: %d–%d", *raw.AgeMin, *raw.AgeMax)
		}
		return "📅 Возраст партнёра: не важен"
	case stepIntent:
		label := intentLabels[raw.RelationshipIntent]
		if label == "" {
			label = raw.RelationshipIntent
		}
		if label == "" {
			return "🎯 Цель знакомства: не указана"
		}
		return "🎯 Цель знакомства: " + label
	case stepEvening:
		return "🌙 Идеальный вечер: " + truncateText(raw.PromptIdealEvening, 120)
	case stepValues:
		return "💎 Ценности: " + truncateText(raw.PromptRelationshipValues, 120)
	case stepInterests:
		return "🎨 Интересы: " + truncateText(raw.PromptOccupation, 120)
	default:
		return step
	}
}

func truncateText(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "не указано"
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func promptReviewStep(ctx context.Context, token string, chatID int64, step string, raw domain.RawProfile) error {
	text := fmt.Sprintf("%s\n\n%s", stepTitle(step), formatStepValue(raw, step))
	return sendMessageWithKeyboard(ctx, token, chatID, text+"\n\nОставить или изменить?", reviewKeyboard(step))
}

func reviewKeyboard(step string) string {
	return fmt.Sprintf(`{"inline_keyboard":[[{"text":"✏️ Изменить","callback_data":"profile:review:edit:%s"},{"text":"➡️ Оставить","callback_data":"profile:review:keep:%s"}]]}`, step, step)
}

func nextStep(step string) string {
	for i, s := range profileStepOrder {
		if s == step && i+1 < len(profileStepOrder) {
			return profileStepOrder[i+1]
		}
	}
	return ""
}

func advanceReview(ctx context.Context, client *apiClient, states *stateStore, token string, chatID int64, userID, currentStep string) error {
	next := nextStep(currentStep)
	if next == "" {
		return completeProfileEdit(ctx, client, states, token, chatID, userID)
	}
	states.set(chatID, next, "review")
	raw, err := loadRawProfile(ctx, client, userID)
	if err != nil {
		return sendWithMainMenu(ctx, token, chatID, "Ошибка загрузки профиля: "+err.Error())
	}
	return promptReviewStep(ctx, token, chatID, next, raw)
}

func completeProfileEdit(ctx context.Context, client *apiClient, states *stateStore, token string, chatID int64, userID string) error {
	if !states.dirty(chatID) {
		states.clear(chatID)
		return sendWithMainMenu(ctx, token, chatID, "Профиль без изменений.")
	}
	states.clear(chatID)
	return finishProfile(ctx, client, token, chatID, userID)
}

func promptCreateStep(ctx context.Context, token string, chatID int64, step string, raw domain.RawProfile) error {
	switch step {
	case stepName:
		return sendMessage(ctx, token, chatID, "Как вас зовут?")
	case stepAge:
		return sendMessage(ctx, token, chatID, "Сколько вам лет?")
	case stepCity:
		return sendMessage(ctx, token, chatID, "В каком вы городе?")
	case stepGender:
		return sendMessageWithKeyboard(ctx, token, chatID, "Ваш пол:", genderKeyboard())
	case stepSeeking:
		return sendSeekingPrompt(ctx, nil, token, chatID, "", raw)
	case stepAgeRange:
		return sendMessage(ctx, token, chatID, ageRangePrompt())
	case stepIntent:
		return sendMessageWithKeyboard(ctx, token, chatID, "Какая у вас цель знакомства?", intentKeyboard())
	case stepEvening:
		return sendMessage(ctx, token, chatID, "Опишите идеальный вечер — 2–4 предложения.\nНапример: пробежка, ужин дома, кино с друзьями…")
	case stepValues:
		return sendMessage(ctx, token, chatID, "Что для вас важно в отношениях?\nЧестность, юмор, общие планы, свобода, семья…")
	case stepInterests:
		return sendMessage(ctx, token, chatID, interestsPrompt())
	default:
		return sendMessage(ctx, token, chatID, "Продолжаем анкету…")
	}
}

func afterStepSaved(ctx context.Context, client *apiClient, states *stateStore, token string, chatID int64, userID, step string) error {
	mode := states.mode(chatID)
	switch mode {
	case "review", "edit":
		return advanceReview(ctx, client, states, token, chatID, userID, step)
	case "create":
		next := nextStep(step)
		if next == "" {
			states.clear(chatID)
			return finishProfile(ctx, client, token, chatID, userID)
		}
		states.set(chatID, next, "create")
		raw, err := loadRawProfile(ctx, client, userID)
		if err != nil {
			return sendMessage(ctx, token, chatID, "Ошибка загрузки профиля: "+err.Error())
		}
		return promptCreateStep(ctx, token, chatID, next, raw)
	default:
		states.clear(chatID)
		return finishProfile(ctx, client, token, chatID, userID)
	}
}

func loadProfile(ctx context.Context, client *apiClient, userID string) (domain.Profile, error) {
	var prof domain.Profile
	if err := client.getJSON(ctx, "/users/"+userID+"/profile", &prof); err != nil {
		return domain.Profile{}, err
	}
	return prof, nil
}
