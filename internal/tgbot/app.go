package tgbot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"karting-bot/internal/config"
	"karting-bot/internal/models"
	"karting-bot/internal/payments"
	"karting-bot/internal/sheets"
	"karting-bot/internal/util"
)

type App struct {
	cfg config.Config
	bot *tgbotapi.BotAPI
	sh  *sheets.Client
	pay payments.PaymentProvider

	// very simple in-memory state machine for registration / admin flows
	state map[int64]userState
}

type userState struct {
	Flow string
	Step int
	Data map[string]string
}

func New(cfg config.Config, sh *sheets.Client, pay payments.PaymentProvider) (*App, error) {
	b, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, err
	}
	b.Debug = false
	return &App{
		cfg:   cfg,
		bot:   b,
		sh:    sh,
		pay:   pay,
		state: map[int64]userState{},
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := a.bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case upd := <-updates:
			if upd.Message != nil {
				if err := a.handleMessage(ctx, upd.Message); err != nil {
					log.Printf("handle msg: %v", err)
				}
			} else if upd.CallbackQuery != nil {
				if err := a.handleCallback(ctx, upd.CallbackQuery); err != nil {
					log.Printf("handle cb: %v", err)
				}
			}
		}
	}
}

func (a *App) SendText(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := a.bot.Send(msg)
	return err
}

func (a *App) isAdmin(tgID int64) bool {
	return a.cfg.AdminTGIDs[tgID]
}

// ---------- Message handling ----------

func (a *App) handleMessage(ctx context.Context, m *tgbotapi.Message) error {
	tgID := m.From.ID
	txt := strings.TrimSpace(m.Text)

	if strings.HasPrefix(txt, "/start") {
		a.state[tgID] = userState{}
		return a.showStart(ctx, tgID)
	}
	if strings.HasPrefix(txt, "/admin") {
		if !a.isAdmin(tgID) {
			return a.SendText(tgID, "Доступ запрещён.")
		}
		a.state[tgID] = userState{}
		return a.showAdminMenu(tgID)
	}

	// flow-based input
	st := a.state[tgID]
	if st.Flow != "" {
		return a.handleFlowInput(ctx, tgID, txt, st)
	}

	// default: show main menu
	return a.showMainMenu(ctx, tgID)
}

func (a *App) handleFlowInput(ctx context.Context, tgID int64, txt string, st userState) error {
	switch st.Flow {
	case "reg":
		return a.handleRegistrationFlow(ctx, tgID, txt, st)
	case "team_create":
		return a.handleTeamCreateFlow(ctx, tgID, txt, st)
	case "admin_create_stage":
		return a.handleAdminCreateStageFlow(ctx, tgID, txt, st)
	case "admin_broadcast":
		return a.handleAdminBroadcastFlow(ctx, tgID, txt, st)
	default:
		a.state[tgID] = userState{}
		return a.SendText(tgID, "Сброс состояния. Нажми /start")
	}
}

func (a *App) showStart(ctx context.Context, tgID int64) error {
	p, _, err := a.sh.GetParticipant(tgID)
	if err != nil {
		return err
	}
	if p == nil {
		// start registration
		a.state[tgID] = userState{Flow: "reg", Step: 1, Data: map[string]string{}}
		return a.SendText(tgID, "Привет! Давай зарегистрируемся. Введи Имя:")
	}
	return a.showProfile(ctx, tgID, p)
}

func (a *App) showProfile(ctx context.Context, tgID int64, p *models.Participant) error {
	text := fmt.Sprintf("👤 Профиль:\n Имя: %s %s\n Ник: %s\n Команда: %s",
		p.FirstName, p.LastName, p.Nick, p.TeamName,
	)
	msg := tgbotapi.NewMessage(tgID, text)
	msg.ParseMode = "Markdown"

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏁 Записаться на этап", "u:stages"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 Сменить команду", "u:change_team"),
			tgbotapi.NewInlineKeyboardButtonData("📅 Календарь", "u:calendar"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏆 Результаты", "u:results"),
			tgbotapi.NewInlineKeyboardButtonData("📸 Фото", "u:photos"),
		),
	)
	msg.ReplyMarkup = kb
	_, err := a.bot.Send(msg)
	return err
}

func (a *App) showMainMenu(ctx context.Context, tgID int64) error {
	p, _, err := a.sh.GetParticipant(tgID)
	if err != nil {
		return err
	}
	if p == nil {
		return a.SendText(tgID, "Ты ещё не зарегистрирован. Нажми /start")
	}
	return a.showProfile(ctx, tgID, p)
}

// ---------- Callback handling ----------

func (a *App) handleCallback(ctx context.Context, q *tgbotapi.CallbackQuery) error {
	tgID := q.From.ID
	data := q.Data

	// ack
	cb := tgbotapi.NewCallback(q.ID, "")
	_, _ = a.bot.Request(cb)

	if strings.HasPrefix(data, "u:") {
		return a.handleUserCallback(ctx, tgID, data)
	}
	if strings.HasPrefix(data, "a:") {
		if !a.isAdmin(tgID) {
			return a.SendText(tgID, "Доступ запрещён.")
		}
		return a.handleAdminCallback(ctx, tgID, data)
	}
	return nil
}

func (a *App) handleUserCallback(ctx context.Context, tgID int64, data string) error {
	switch data {
	case "u:stages":
		return a.showStages(ctx, tgID, true)
	case "u:calendar":
		return a.showStages(ctx, tgID, false)
	case "u:change_team":
		return a.showTeamPicker(ctx, tgID)
	case "u:results":
		return a.showStagesForResults(ctx, tgID)
	case "u:photos":
		return a.showStagesForPhotos(ctx, tgID)
	}

	if strings.HasPrefix(data, "u:reg_team:") {
		team := strings.TrimPrefix(data, "u:reg_team:")
		return a.handleUserCallbackRegTeam(ctx, tgID, team)
	}

	if strings.HasPrefix(data, "u:pick_team:") {
		name := strings.TrimPrefix(data, "u:pick_team:")
		if name == "__create__" {
			a.state[tgID] = userState{Flow: "team_create", Step: 1, Data: map[string]string{}}
			return a.SendText(tgID, "Введи название новой команды:")
		}
		if err := a.sh.UpdateParticipantTeam(tgID, name); err != nil {
			return err
		}
		return a.SendText(tgID, "✅ Команда обновлена: "+name+" Нажми /start")
	}

	if strings.HasPrefix(data, "u:stage_join:") {
		stageID := strings.TrimPrefix(data, "u:stage_join:")
		return a.joinStage(ctx, tgID, stageID)
	}

	if strings.HasPrefix(data, "u:pay:") {
		// u:pay:<stage_id>
		stageID := strings.TrimPrefix(data, "u:pay:")
		return a.startPayment(ctx, tgID, stageID)
	}

	if strings.HasPrefix(data, "u:result_stage:") {
		stageID := strings.TrimPrefix(data, "u:result_stage:")
		return a.showResult(ctx, tgID, stageID)
	}

	if strings.HasPrefix(data, "u:photo_stage:") {
		stageID := strings.TrimPrefix(data, "u:photo_stage:")
		return a.showPhoto(ctx, tgID, stageID)
	}

	return nil
}

func (a *App) handleAdminCallback(ctx context.Context, tgID int64, data string) error {
	switch data {
	case "a:menu":
		return a.showAdminMenu(tgID)
	case "a:create_stage":
		a.state[tgID] = userState{Flow: "admin_create_stage", Step: 1, Data: map[string]string{}}
		return a.SendText(tgID, "Создание этапа. Введи stage_id (например: 1 или st1):")
	case "a:list_stages":
		return a.showStages(ctx, tgID, false)
	case "a:broadcast":
		a.state[tgID] = userState{Flow: "admin_broadcast", Step: 1, Data: map[string]string{}}
		return a.SendText(tgID, "Рассылка. Введи текст сообщения (будет отправлено всем зарегистрированным):")
	}

	if strings.HasPrefix(data, "a:toggle_reg:") {
		// a:toggle_reg:<stage_id>
		stageID := strings.TrimPrefix(data, "a:toggle_reg:")
		st, err := a.sh.GetStage(stageID)
		if err != nil {
			return err
		}
		if st == nil {
			return a.SendText(tgID, "Этап не найден")
		}
		open := !util.NormalizeBoolRU(st.RegOpen)
		if err := a.sh.SetStageRegOpen(stageID, open); err != nil {
			return err
		}
		if open {
			return a.SendText(tgID, "✅ Регистрация открыта для этапа "+stageID)
		}
		return a.SendText(tgID, "✅ Регистрация закрыта для этапа "+stageID)
	}

	if strings.HasPrefix(data, "a:export:") {
		stageID := strings.TrimPrefix(data, "a:export:")
		token := util.HMACSHA256Hex(a.cfg.PaymentWebhookSecret, "export:"+stageID)
		url := a.cfg.BasePublicURL + "/export/stage.csv?stage_id=" + stageID + "&token=" + token
		if a.cfg.BasePublicURL == "" {
			url = "http://localhost" + a.cfg.HTTPAddr + "/export/stage.csv?stage_id=" + stageID + "&token=" + token
		}
		return a.SendText(tgID, "📤 CSV выгрузка (ссылка): "+url)
	}

	return nil
}

// ---------- Screens / Menus ----------

func (a *App) showAdminMenu(tgID int64) error {
	msg := tgbotapi.NewMessage(tgID, "🛠 *Админ-панель*")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Создать этап", "a:create_stage"),
			tgbotapi.NewInlineKeyboardButtonData("📋 Список этапов", "a:list_stages"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📢 Рассылка всем", "a:broadcast"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏠 В меню", "u:calendar"),
		),
	)
	_, err := a.bot.Send(msg)
	return err
}

func (a *App) showStages(ctx context.Context, tgID int64, onlyOpen bool) error {
	stages, err := a.sh.ListStages(!onlyOpen)
	if err != nil {
		return err
	}
	if len(stages) == 0 {
		if onlyOpen {
			return a.SendText(tgID, "Сейчас нет этапов с открытой регистрацией.")
		}
		return a.SendText(tgID, "Этапов пока нет.")
	}

	text := "🏁 Этапы"
	for _, s := range stages {
		open := "закрыта"
		if util.NormalizeBoolRU(s.RegOpen) {
			open = "открыта"
		}
		text += fmt.Sprintf("*%s* (id: `%s`)\n 📅 %s %s\n 📍 %s\n Регистрация: %s\n Цена: %s",
			s.Title, s.StageID, s.Date, s.Time, s.Place, open, s.Price,
		)
		if strings.TrimSpace(s.Address) != "" {
			text += "Адрес: " + s.Address + ""
		}
	}

	msg := tgbotapi.NewMessage(tgID, text)
	msg.ParseMode = "Markdown"

	// build keyboard
	rows := [][]tgbotapi.InlineKeyboardButton{}
	for _, s := range stages {
		if onlyOpen && !util.NormalizeBoolRU(s.RegOpen) {
			continue
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏁 Записаться: "+s.Title, "u:stage_join:"+s.StageID),
		))
		if a.isAdmin(tgID) {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔓/🔒 Регистрация", "a:toggle_reg:"+s.StageID),
				tgbotapi.NewInlineKeyboardButtonData("📤 CSV", "a:export:"+s.StageID),
			))
		}
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🏠 В профиль", "u:calendar"),
	))
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err = a.bot.Send(msg)
	return err
}

func (a *App) showTeamPicker(ctx context.Context, tgID int64) error {
	teams, err := a.sh.ListTeams()
	if err != nil {
		return err
	}
	rows := [][]tgbotapi.InlineKeyboardButton{}
	for _, t := range teams {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(t.TeamName, "u:pick_team:"+t.TeamName),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ Создать новую", "u:pick_team:__create__"),
	))
	msg := tgbotapi.NewMessage(tgID, "Выбери команду:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err = a.bot.Send(msg)
	return err
}

// ---------- Actions ----------

func (a *App) joinStage(ctx context.Context, tgID int64, stageID string) error {
	st, err := a.sh.GetStage(stageID)
	if err != nil {
		return err
	}
	if st == nil {
		return a.SendText(tgID, "Этап не найден.")
	}
	if !util.NormalizeBoolRU(st.RegOpen) {
		return a.SendText(tgID, "Регистрация на этот этап закрыта.")
	}

	has, err := a.sh.HasRegistration(stageID, tgID)
	if err != nil {
		return err
	}
	if has {
		return a.SendText(tgID, "Ты уже записан на этот этап.")
	}

	p, _, err := a.sh.GetParticipant(tgID)
	if err != nil {
		return err
	}
	if p == nil {
		return a.SendText(tgID, "Сначала зарегистрируйся: /start")
	}

	cnt, err := a.sh.CountMainForTeam(stageID, p.TeamName)
	if err != nil {
		return err
	}
	role := "main"
	if cnt >= 3 {
		role = "reserve"
	}

	reg := models.Registration{
		StageID:   stageID,
		TgID:      tgID,
		TeamName:  p.TeamName,
		Role:      role,
		PayStatus: "unpaid",
		CreatedAt: util.NowISO(),
	}
	if err := a.sh.CreateRegistration(reg); err != nil {
		return err
	}

	txt := "✅ Запись создана.\n Статус: *" + role + "*\n Теперь нужно оплатить участие."
	if role == "reserve" {
		txt = "✅ Запись создана. ⚠️ Ты записан в *резерв* (в команде уже 3 основных пилота). Оплата доступна, но участие зависит от освобождения места."
	}
	msg := tgbotapi.NewMessage(tgID, txt)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💳 Оплатить", "u:pay:"+stageID),
		),
	)
	_, err = a.bot.Send(msg)
	return err
}

func (a *App) startPayment(ctx context.Context, tgID int64, stageID string) error {
	st, err := a.sh.GetStage(stageID)
	if err != nil {
		return err
	}
	if st == nil {
		return a.SendText(tgID, "Этап не найден.")
	}

	amount := strings.TrimSpace(st.Price)
	if amount == "" {
		amount = "0"
	}

	returnURL := ""
	payURL, _, err := a.pay.CreatePayment(ctx, stageID, tgID, amount, returnURL)
	if err != nil {
		return err
	}

	txt := fmt.Sprintf(
		"Оплата этапа *%s* (id: `%s`)\nСумма: *%s*\n\nПерейди по ссылке:\n%s\n\nПосле оплаты бот сам подтвердит статус.",
		st.Title, st.StageID, amount, payURL,
	)

	msg := tgbotapi.NewMessage(tgID, txt)
	msg.ParseMode = "Markdown"
	_, err = a.bot.Send(msg)
	return err
}

// ---------- Results / Photos ----------

func (a *App) showStagesForResults(ctx context.Context, tgID int64) error {
	stages, err := a.sh.ListStages(true)
	if err != nil {
		return err
	}
	if len(stages) == 0 {
		return a.SendText(tgID, "Этапов пока нет.")
	}
	rows := [][]tgbotapi.InlineKeyboardButton{}
	for _, s := range stages {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(s.Title, "u:result_stage:"+s.StageID),
		))
	}
	msg := tgbotapi.NewMessage(tgID, "Выбери этап для результатов:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err = a.bot.Send(msg)
	return err
}

func (a *App) showResult(ctx context.Context, tgID int64, stageID string) error {
	res, err := a.sh.GetResult(stageID, tgID)
	if err != nil {
		return err
	}
	if res == nil {
		return a.SendText(tgID, "Результатов по этому этапу пока нет.")
	}
	sum, _ := a.sh.SumPointsForUser(tgID)
	txt := fmt.Sprintf("🏆 Результаты (этап `%s`)\n Лучшее время: *%s*\n Позиция: *%s*\n Очки за этап: *%s*\n Очки за сезон (всего): *%d*",
		stageID, res.BestTime, res.Position, res.Points, sum,
	)
	msg := tgbotapi.NewMessage(tgID, txt)
	msg.ParseMode = "Markdown"
	_, err = a.bot.Send(msg)
	return err
}

func (a *App) showStagesForPhotos(ctx context.Context, tgID int64) error {
	stages, err := a.sh.ListStages(true)
	if err != nil {
		return err
	}
	if len(stages) == 0 {
		return a.SendText(tgID, "Этапов пока нет.")
	}
	rows := [][]tgbotapi.InlineKeyboardButton{}
	for _, s := range stages {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(s.Title, "u:photo_stage:"+s.StageID),
		))
	}
	msg := tgbotapi.NewMessage(tgID, "Выбери этап для фото:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err = a.bot.Send(msg)
	return err
}

func (a *App) showPhoto(ctx context.Context, tgID int64, stageID string) error {
	ph, err := a.sh.GetPhoto(stageID)
	if err != nil {
		return err
	}
	if ph == nil || strings.TrimSpace(ph.URL) == "" {
		return a.SendText(tgID, "Фото по этому этапу пока не добавлено.")
	}
	return a.SendText(tgID, "📸 Фото этапа:"+ph.URL)
}

// ---------- CSV export builder ----------

func (a *App) BuildStageCSV(ctx context.Context, stageID string) (string, error) {
	regs, err := a.sh.ListRegistrationsForStage(stageID)
	if err != nil {
		return "", err
	}
	if len(regs) == 0 {
		return "team,first_name,last_name,nick,role,pay_status", nil
	}

	// We need participant info per tg
	header := "team,first_name,last_name,nick,role,pay_status"
	b := strings.Builder{}
	b.WriteString(header)
	for _, r := range regs {
		p, _, err := a.sh.GetParticipant(r.TgID)
		if err != nil {
			return "", err
		}
		if p == nil {
			continue
		}
		line := fmt.Sprintf("%s,%s,%s,%s,%s,%s",
			escapeCSV(r.TeamName),
			escapeCSV(p.FirstName),
			escapeCSV(p.LastName),
			escapeCSV(p.Nick),
			escapeCSV(r.Role),
			escapeCSV(r.PayStatus),
		)
		b.WriteString(line)
	}
	return b.String(), nil
}

func escapeCSV(s string) string {
	s = strings.ReplaceAll(s, `"`, `""`)
	if strings.ContainsAny(s, ",\n\r") {
		return `"` + s + `"`
	}
	return s
}

// ---------- Flows ----------

func (a *App) handleRegistrationFlow(ctx context.Context, tgID int64, txt string, st userState) error {
	if st.Data == nil {
		st.Data = map[string]string{}
	}

	switch st.Step {
	case 1:
		st.Data["first_name"] = txt
		st.Step = 2
		a.state[tgID] = st
		return a.SendText(tgID, "Введи фамилию:")
	case 2:
		st.Data["last_name"] = txt
		st.Step = 3
		a.state[tgID] = st
		return a.SendText(tgID, "Введи ник (как тебя подписывать в чемпионате):")
	case 3:
		st.Data["nick"] = txt
		// next: team selection via keyboard
		a.state[tgID] = st
		return a.showTeamPickerForRegistration(ctx, tgID)
	default:
		a.state[tgID] = userState{}
		return a.SendText(tgID, "Регистрация завершена. /start")
	}
}

func (a *App) showTeamPickerForRegistration(ctx context.Context, tgID int64) error {
	teams, err := a.sh.ListTeams()
	if err != nil {
		return err
	}

	rows := [][]tgbotapi.InlineKeyboardButton{}
	for _, t := range teams {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(t.TeamName, "u:reg_team:"+t.TeamName),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ Создать новую", "u:reg_team:__create__"),
	))

	msg := tgbotapi.NewMessage(tgID, "Выбери команду или создай новую:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err = a.bot.Send(msg)
	return err
}

// We reuse callback handler: add support for u:reg_team:
func (a *App) handleUserCallbackRegTeam(ctx context.Context, tgID int64, team string) error {
	st := a.state[tgID]
	if st.Flow != "reg" {
		return a.SendText(tgID, "Нажми /start")
	}
	if team == "__create__" {
		a.state[tgID] = userState{Flow: "team_create", Step: 1, Data: map[string]string{"after": "reg"}}
		return a.SendText(tgID, "Введи название новой команды:")
	}
	// finalize registration
	p := models.Participant{
		TgID:      tgID,
		FirstName: st.Data["first_name"],
		LastName:  st.Data["last_name"],
		Nick:      st.Data["nick"],
		TeamName:  team,
		CreatedAt: util.NowISO(),
	}
	if err := a.sh.CreateParticipant(p); err != nil {
		return err
	}
	a.state[tgID] = userState{}
	return a.SendText(tgID, "✅ Регистрация завершена! Нажми /start")
}

func (a *App) handleTeamCreateFlow(ctx context.Context, tgID int64, txt string, st userState) error {
	name := strings.TrimSpace(txt)
	if name == "" {
		return a.SendText(tgID, "Название не может быть пустым. Введи ещё раз:")
	}
	_, err := a.sh.CreateTeam(name)
	if err != nil {
		return err
	}

	after := ""
	if st.Data != nil {
		after = st.Data["after"]
	}

	if after == "reg" {
		// go back to reg finalize: set team and create participant
		// we need to simulate callback
		return a.handleUserCallbackRegTeam(ctx, tgID, name)
	}

	// otherwise: just set in profile
	if err := a.sh.UpdateParticipantTeam(tgID, name); err != nil {
		return err
	}
	a.state[tgID] = userState{}
	return a.SendText(tgID, "✅ Команда создана и выбрана: "+name+" Нажми /start")
}

func (a *App) handleAdminCreateStageFlow(ctx context.Context, tgID int64, txt string, st userState) error {
	if st.Data == nil {
		st.Data = map[string]string{}
	}
	switch st.Step {
	case 1:
		st.Data["stage_id"] = strings.TrimSpace(txt)
		if st.Data["stage_id"] == "" {
			return a.SendText(tgID, "stage_id пустой. Введи ещё раз:")
		}
		st.Step = 2
		a.state[tgID] = st
		return a.SendText(tgID, "Название этапа:")
	case 2:
		st.Data["title"] = txt
		st.Step = 3
		a.state[tgID] = st
		return a.SendText(tgID, "Дата (например 2026-03-10):")
	case 3:
		st.Data["date"] = txt
		st.Step = 4
		a.state[tgID] = st
		return a.SendText(tgID, "Время (например 18:00):")
	case 4:
		st.Data["time"] = txt
		st.Step = 5
		a.state[tgID] = st
		return a.SendText(tgID, "Место (клуб/трасса):")
	case 5:
		st.Data["place"] = txt
		st.Step = 6
		a.state[tgID] = st
		return a.SendText(tgID, "Адрес (можно со ссылкой на карты):")
	case 6:
		st.Data["address"] = txt
		st.Step = 7
		a.state[tgID] = st
		return a.SendText(tgID, "Цена (число, например 1500):")
	case 7:
		st.Data["price"] = txt
		// default reg_open = нет; admin can open later
		s := models.Stage{
			StageID: st.Data["stage_id"],
			Title:   st.Data["title"],
			Date:    st.Data["date"],
			Time:    st.Data["time"],
			Place:   st.Data["place"],
			Address: st.Data["address"],
			RegOpen: "нет",
			Price:   st.Data["price"],
		}
		if err := a.sh.CreateStage(s); err != nil {
			return err
		}
		a.state[tgID] = userState{}
		return a.SendText(tgID, "✅ Этап создан. Регистрация по умолчанию закрыта. Нажми /admin")
	default:
		a.state[tgID] = userState{}
		return a.SendText(tgID, "Сброс. /admin")
	}
}

func (a *App) handleAdminBroadcastFlow(ctx context.Context, tgID int64, txt string, st userState) error {
	msgText := strings.TrimSpace(txt)
	if msgText == "" {
		return a.SendText(tgID, "Текст пустой. Введи ещё раз:")
	}
	// broadcast to all participants
	ids, err := a.sh.ListParticipantIDs()
	if err != nil {
		return err
	}
	sent := 0
	for _, id := range ids {
		_ = a.SendText(id, "📢 Сообщение от организаторов: "+msgText)
		sent++
		time.Sleep(35 * time.Millisecond) // simple anti-flood
	}
	a.state[tgID] = userState{}
	return a.SendText(tgID, fmt.Sprintf("✅ Рассылка выполнена: %d получателей.", sent))
}
