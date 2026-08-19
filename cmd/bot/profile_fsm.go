package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/alterwalker/test_dating_bot/internal/domain"
)

func handleProfileStep(ctx context.Context, client *apiClient, states *stateStore, token string, chatID int64, userID, text string) error {
	step := states.step(chatID)
	if step == "" {
		return nil
	}
	mode := states.mode(chatID)
	if mode != "create" && mode != "edit" {
		return nil
	}

	raw, err := loadRawProfile(ctx, client, userID)
	if err != nil {
		return sendMessage(ctx, token, chatID, "Ошибка загрузки профиля: "+err.Error())
	}

	switch step {
	case stepName:
		raw.Name = text
	case stepAge:
		age, err := strconv.Atoi(text)
		if err != nil || age < 18 || age > 99 {
			return sendMessage(ctx, token, chatID, "Введите возраст числом от 18 до 99")
		}
		raw.Age = age
	case stepCity:
		raw.City = text
	case stepGender:
		return sendMessageWithKeyboard(ctx, token, chatID, "Выберите пол кнопкой ниже:", genderKeyboard())
	case stepSeeking:
		return sendSeekingPrompt(ctx, client, token, chatID, userID, raw)
	case stepAgeRange:
		min, max, skipped, err := parseAgeRange(text, raw.Age)
		if err != nil {
			return sendMessage(ctx, token, chatID, err.Error()+"\n\nПример: 25-35 или «пропустить»")
		}
		if skipped {
			raw.AgeMin = nil
			raw.AgeMax = nil
		} else {
			raw.AgeMin = min
			raw.AgeMax = max
		}
	case stepIntent:
		return sendMessageWithKeyboard(ctx, token, chatID, "Выберите цель знакомства кнопкой ниже:", intentKeyboard())
	case stepEvening:
		if len(strings.TrimSpace(text)) < 10 {
			return sendMessage(ctx, token, chatID, "Напишите чуть подробнее — 2–4 предложения об идеальном вечере.")
		}
		raw.PromptIdealEvening = text
	case stepValues:
		if len(strings.TrimSpace(text)) < 10 {
			return sendMessage(ctx, token, chatID, "Напишите чуть подробнее — что для вас важно в отношениях.")
		}
		raw.PromptRelationshipValues = text
	case stepInterests:
		if len(strings.TrimSpace(text)) < 10 {
			return sendMessage(ctx, token, chatID, "Напишите чуть подробнее — ваши интересы помимо работы.")
		}
		raw.PromptOccupation = text
	default:
		return nil
	}

	if err := saveRawProfile(ctx, client, userID, raw); err != nil {
		return sendMessage(ctx, token, chatID, "Ошибка сохранения: "+err.Error())
	}
	if mode == "edit" {
		states.markDirty(chatID)
	}
	return afterStepSaved(ctx, client, states, token, chatID, userID, step)
}

func handleProfileCallback(ctx context.Context, client *apiClient, states *stateStore, token string, chatID int64, userID, data string) error {
	parts := strings.Split(data, ":")
	if len(parts) < 3 || parts[0] != "profile" {
		return nil
	}

	raw, err := loadRawProfile(ctx, client, userID)
	if err != nil {
		return sendMessage(ctx, token, chatID, "Ошибка загрузки профиля: "+err.Error())
	}

	switch parts[1] {
	case "delete":
		if len(parts) < 3 {
			return nil
		}
		switch parts[2] {
		case "ask":
			return sendMessageWithKeyboard(ctx, token, chatID,
				"Удалить анкету безвозвратно?\n\nБудут удалены все поля, AI-разметка и embedding. В matches вы больше не будете участвовать.",
				profileDeleteConfirmKeyboard())
		case "yes":
			return deleteProfile(ctx, client, states, token, chatID, userID)
		case "cancel":
			return sendWithMainMenu(ctx, token, chatID, "Удаление отменено.")
		}
		return nil

	case "review":
		if len(parts) < 4 {
			return nil
		}
		action, step := parts[2], parts[3]
		if states.mode(chatID) != "review" || states.step(chatID) != step {
			return nil
		}
		switch action {
		case "keep":
			return advanceReview(ctx, client, states, token, chatID, userID, step)
		case "edit":
			states.set(chatID, step, "edit")
			hint := fmt.Sprintf("✏️ Изменяем: %s\n%s\n\n", stepTitle(step), formatStepValue(raw, step))
			if err := sendMessage(ctx, token, chatID, hint); err != nil {
				return err
			}
			return promptCreateStep(ctx, token, chatID, step, raw)
		}
		return nil

	case "gender":
		if states.step(chatID) != stepGender {
			return nil
		}
		if mode := states.mode(chatID); mode != "create" && mode != "edit" {
			return nil
		}
		gender := parts[2]
		if gender != "male" && gender != "female" {
			return nil
		}
		raw.Gender = gender
		if mode := states.mode(chatID); mode == "edit" {
			raw.Seeking = nil
		}
		if err := saveRawProfile(ctx, client, userID, raw); err != nil {
			return sendMessage(ctx, token, chatID, "Ошибка сохранения: "+err.Error())
		}
		if states.mode(chatID) == "edit" {
			states.markDirty(chatID)
		}
		if states.mode(chatID) == "create" {
			states.set(chatID, stepSeeking, "create")
			return sendSeekingPrompt(ctx, client, token, chatID, userID, raw)
		}
		return afterStepSaved(ctx, client, states, token, chatID, userID, stepGender)

	case "seeking":
		if states.step(chatID) != stepSeeking {
			return nil
		}
		if mode := states.mode(chatID); mode != "create" && mode != "edit" {
			return nil
		}
		if parts[2] == "done" {
			if len(raw.Seeking) == 0 {
				return sendMessage(ctx, token, chatID, "Выберите хотя бы один вариант, кого вы ищете.")
			}
			if states.mode(chatID) == "edit" {
				states.markDirty(chatID)
			}
			return afterStepSaved(ctx, client, states, token, chatID, userID, stepSeeking)
		}
		if parts[2] != "toggle" || len(parts) < 4 {
			return nil
		}
		target := parts[3]
		if target != "male" && target != "female" {
			return nil
		}
		raw.Seeking = toggleSeeking(raw.Seeking, target)
		if err := saveRawProfile(ctx, client, userID, raw); err != nil {
			return sendMessage(ctx, token, chatID, "Ошибка сохранения: "+err.Error())
		}
		if states.mode(chatID) == "edit" {
			states.markDirty(chatID)
		}
		return sendSeekingPrompt(ctx, client, token, chatID, userID, raw)

	case "intent":
		if states.step(chatID) != stepIntent {
			return nil
		}
		if mode := states.mode(chatID); mode != "create" && mode != "edit" {
			return nil
		}
		intent := parts[2]
		if _, ok := intentLabels[intent]; !ok {
			return nil
		}
		raw.RelationshipIntent = intent
		if err := saveRawProfile(ctx, client, userID, raw); err != nil {
			return sendMessage(ctx, token, chatID, "Ошибка сохранения: "+err.Error())
		}
		if states.mode(chatID) == "edit" {
			states.markDirty(chatID)
		}
		return afterStepSaved(ctx, client, states, token, chatID, userID, stepIntent)
	}

	return nil
}

func sendSeekingPrompt(ctx context.Context, client *apiClient, token string, chatID int64, userID string, raw domain.RawProfile) error {
	_ = client
	_ = userID
	text := "Кого вы ищете? Можно выбрать несколько вариантов, затем нажмите «Готово»."
	if len(raw.Seeking) > 0 {
		text += "\n\nВыбрано: " + formatSeeking(raw.Seeking)
	}
	return sendMessageWithKeyboard(ctx, token, chatID, text, seekingKeyboard(raw.Seeking))
}

func interestsPrompt() string {
	return "Расскажите о ваших интересах помимо работы — хобби, спорт, увлечения.\n2–4 предложения, всё что считаете важным."
}

func ageRangePrompt() string {
	return "Какой возраст вас интересует?\n\nНапишите диапазон, например: 25-35\nИли «пропустить», если возраст не важен."
}

func formatSeeking(seeking []string) string {
	labels := map[string]string{
		"male":   "мужчин",
		"female": "женщин",
	}
	var parts []string
	for _, s := range seeking {
		if label, ok := labels[s]; ok {
			parts = append(parts, label)
		}
	}
	return strings.Join(parts, ", ")
}

func toggleSeeking(current []string, target string) []string {
	for i, s := range current {
		if s == target {
			return append(current[:i], current[i+1:]...)
		}
	}
	return append(current, target)
}

func parseAgeRange(text string, userAge int) (min, max *int, skipped bool, err error) {
	t := strings.ToLower(strings.TrimSpace(text))
	t = strings.ReplaceAll(t, "—", "-")
	t = strings.ReplaceAll(t, "–", "-")
	if t == "пропустить" || t == "skip" || t == "-" || t == "нет" {
		return nil, nil, true, nil
	}

	parts := strings.FieldsFunc(t, func(r rune) bool {
		return r == '-' || r == ' ' || r == ','
	})
	if len(parts) != 2 {
		return nil, nil, false, fmt.Errorf("укажите диапазон в формате 25-35")
	}
	ageMin, err1 := strconv.Atoi(parts[0])
	ageMax, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return nil, nil, false, fmt.Errorf("возраст должен быть числом")
	}
	if ageMin > ageMax {
		ageMin, ageMax = ageMax, ageMin
	}
	if ageMin < 18 {
		return nil, nil, false, fmt.Errorf("минимальный возраст — 18")
	}
	if ageMax > 99 {
		return nil, nil, false, fmt.Errorf("максимальный возраст — 99")
	}
	_ = userAge
	return &ageMin, &ageMax, false, nil
}

func genderKeyboard() string {
	return `{"inline_keyboard":[[{"text":"Мужской","callback_data":"profile:gender:male"},{"text":"Женский","callback_data":"profile:gender:female"}]]}`
}

func seekingKeyboard(selected []string) string {
	has := func(v string) bool {
		for _, s := range selected {
			if s == v {
				return true
			}
		}
		return false
	}
	mark := func(v, label string) string {
		if has(v) {
			return "✓ " + label
		}
		return label
	}
	return fmt.Sprintf(`{"inline_keyboard":[[{"text":"%s","callback_data":"profile:seeking:toggle:male"},{"text":"%s","callback_data":"profile:seeking:toggle:female"}],[{"text":"Готово","callback_data":"profile:seeking:done"}]]}`,
		mark("male", "Мужчин"), mark("female", "Женщин"))
}

func intentKeyboard() string {
	return `{"inline_keyboard":[[{"text":"Серьёзные отношения","callback_data":"profile:intent:serious"}],[{"text":"Лёгкое общение","callback_data":"profile:intent:casual"}],[{"text":"Дружба","callback_data":"profile:intent:friendship"}],[{"text":"Пока не знаю","callback_data":"profile:intent:unsure"}]]}`
}

func loadRawProfile(ctx context.Context, client *apiClient, userID string) (domain.RawProfile, error) {
	prof, err := loadProfile(ctx, client, userID)
	if err != nil {
		return domain.RawProfile{}, err
	}
	return prof.Raw, nil
}

func saveRawProfile(ctx context.Context, client *apiClient, userID string, raw domain.RawProfile) error {
	return client.putJSON(ctx, "/users/"+userID+"/profile/raw", raw, nil)
}
