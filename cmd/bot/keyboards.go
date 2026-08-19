package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/alterwalker/test_dating_bot/internal/domain"
)

const (
	btnProfile    = "👤 Профиль"
	btnMatches    = "🔍 Matches"
	btnAdminStats = "📊 Admin статистика"
)

func mainMenuKeyboard() string {
	return `{"keyboard":[[{"text":"` + btnProfile + `"},{"text":"` + btnMatches + `"}],[{"text":"` + btnAdminStats + `"}]],"resize_keyboard":true,"is_persistent":true}`
}

func sendWithMainMenu(ctx context.Context, token string, chatID int64, text string) error {
	return sendMessageWithKeyboard(ctx, token, chatID, text, mainMenuKeyboard())
}

func normalizeMenuText(text string) string {
	switch text {
	case btnProfile:
		return "/profile"
	case btnMatches:
		return "/matches"
	case btnAdminStats:
		return "/admin"
	default:
		return text
	}
}

func isMenuAction(text string) bool {
	switch text {
	case btnProfile, btnMatches, btnAdminStats, "/profile", "/matches", "/admin", "/start", "/cancel":
		return true
	default:
		return false
	}
}

func showAdminStats(ctx context.Context, client *apiClient, token string, chatID int64) error {
	var stats domain.AdminStatsResponse
	if err := client.getJSON(ctx, "/admin/stats/cities", &stats); err != nil {
		return sendWithMainMenu(ctx, token, chatID, "Не удалось загрузить статистику: "+err.Error())
	}
	return sendWithMainMenu(ctx, token, chatID, formatAdminCityStats(stats))
}

func formatAdminCityStats(stats domain.AdminStatsResponse) string {
	var b strings.Builder
	b.WriteString("📊 Admin статистика\n\n")
	b.WriteString(fmt.Sprintf("Анкеты confirmed: %d\n", stats.TotalConfirmed))
	b.WriteString(fmt.Sprintf("👤 Telegram-пользователи: %d\n\n", stats.TelegramUsers))

	b.WriteString("🏙 Города (топ-10)\n")
	if len(stats.Cities) == 0 {
		b.WriteString("Нет данных.\n")
	} else {
		for i, c := range stats.Cities {
			b.WriteString(fmt.Sprintf("%d. %s — %d (М %d · Ж %d)\n", i+1, c.City, c.Total, c.Male, c.Female))
		}
	}

	b.WriteString("\n🤖 Токены OpenAI\n")
	if len(stats.TokenByModel) == 0 && len(stats.TokenUsage) == 0 {
		b.WriteString("Нет данных (AI_MOCK или миграция 003 не применена).\n")
	} else {
		var grandTotal int64
		for _, m := range stats.TokenByModel {
			grandTotal += m.TotalTokens
		}
		b.WriteString(fmt.Sprintf("Всего: %s\n\n", formatTokenCount(grandTotal)))

		if len(stats.TokenByModel) > 0 {
			b.WriteString("По моделям:\n")
			for _, m := range stats.TokenByModel {
				b.WriteString(fmt.Sprintf("• %s — %s (запросов %d)\n",
					m.Model, formatTokenCount(m.TotalTokens), m.RequestCount))
			}
		}

		if len(stats.TokenUsage) > 0 {
			b.WriteString("\nПо операции и источнику:\n")
			for _, row := range stats.TokenUsage {
				b.WriteString(fmt.Sprintf("• %s / %s [%s] — %s (%d req)\n",
					tokenOpLabel(row.Operation), row.Source,
					row.Model, formatTokenCount(row.TotalTokens), row.RequestCount))
			}
		}
	}

	return strings.TrimSpace(b.String())
}

func formatTokenCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func tokenOpLabel(op string) string {
	switch op {
	case "extract":
		return "extract"
	case "embed":
		return "embed"
	case "explain":
		return "explain"
	case "icebreaker":
		return "icebreaker"
	default:
		return op
	}
}
