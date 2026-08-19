# Icebreaker — user message template

---

## Шаблон

```
Пользователь (кто пишет): {{.ViewerName}}, {{.ViewerAge}} лет
Его/её интересы: {{.ViewerInterests}}
Ценности: {{.ViewerValues}}
О себе: {{.ViewerSummary}}

---

Собеседник: {{.CandidateName}}, {{.CandidateAge}} лет
Интересы: {{.CandidateInterests}}
Ценности: {{.CandidateValues}}
О себе: {{.CandidateSummary}}

---

Общее:
- интересы: {{.SharedInterests}}
- ценности: {{.SharedValues}}

Предложи темы для беседы и одно первое сообщение от имени {{.ViewerName}}.
```

## Пример ответа

```json
{
  "conversation_topics": [
    "Какие дистанции бегаете — больше короткие или полумарафоны?",
    "Как вписываете пробежки в рабочую неделю?",
    "Есть любимые парки или маршруты в Москве?"
  ],
  "opener_message": "Привет! Увидел, что ты тоже бегаешь — давно начал или недавно подсел?"
}
```

## JSON Schema (structured output)

```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["conversation_topics", "opener_message"],
  "properties": {
    "conversation_topics": {
      "type": "array",
      "items": { "type": "string", "minLength": 10, "maxLength": 120 },
      "minItems": 3,
      "maxItems": 5
    },
    "opener_message": {
      "type": "string",
      "minLength": 20,
      "maxLength": 280
    }
  }
}
```

Сохранить как `prompts/icebreaker.schema.json` при реализации.
