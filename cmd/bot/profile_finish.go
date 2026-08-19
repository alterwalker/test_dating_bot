package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alterwalker/test_dating_bot/internal/domain"
)

func finishProfile(ctx context.Context, client *apiClient, token string, chatID int64, userID string) error {
	if err := sendMessage(ctx, token, chatID, "Сохраняю и отправляю на разметку…"); err != nil {
		return err
	}

	var resp map[string]any
	if err := client.postJSON(ctx, "/users/"+userID+"/profile/enrich", nil, &resp); err != nil {
		return sendMessage(ctx, token, chatID, "Ошибка enrich: "+err.Error())
	}
	for i := 0; i < 30; i++ {
		var status map[string]any
		_ = client.getJSON(ctx, "/users/"+userID+"/profile/status", &status)
		if status["status"] == "ready" {
			break
		}
		time.Sleep(time.Second)
	}

	var prof domain.Profile
	if err := client.getJSON(ctx, "/users/"+userID+"/profile", &prof); err != nil {
		return sendMessage(ctx, token, chatID, "Профиль обрабатывается, попробуйте /profile позже")
	}
	if prof.Enriched == nil {
		return sendMessage(ctx, token, chatID, "Не удалось обработать профиль. Попробуйте /profile ещё раз.")
	}

	_ = client.postJSON(ctx, "/users/"+userID+"/profile/confirm", nil, nil)

	var b strings.Builder
	b.WriteString("Профиль готов!\n\n")
	b.WriteString(prof.Enriched.Summary)
	if prof.Raw.RelationshipIntent != "" {
		if label, ok := intentLabels[prof.Raw.RelationshipIntent]; ok {
			b.WriteString("\n\n🎯 Цель: " + label)
		}
	}
	if prof.Raw.AgeMin != nil && prof.Raw.AgeMax != nil {
		b.WriteString(fmt.Sprintf("\n👥 Ищу: %s, %d–%d лет", formatSeeking(prof.Raw.Seeking), *prof.Raw.AgeMin, *prof.Raw.AgeMax))
	} else if len(prof.Raw.Seeking) > 0 {
		b.WriteString("\n👥 Ищу: " + formatSeeking(prof.Raw.Seeking))
	}
	b.WriteString("\n\nНажмите «🔍 Matches», чтобы найти пары.")
	return sendWithMainMenu(ctx, token, chatID, b.String())
}
