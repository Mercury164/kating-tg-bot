package sheets

import (
    "context"
    "fmt"
    "strconv"
    "strings"

    sheetsv4 "google.golang.org/api/sheets/v4"

    "karting-bot/internal/models"
    "karting-bot/internal/util"
)

const (
    SheetParticipants       = "Participants"
    SheetTeams              = "Teams"
    SheetStages             = "Stages"
    SheetRegistrations      = "Stage_Registrations"
    SheetResults            = "Results"
    SheetPhotos             = "Photos"
)

func (c *Client) readAll(sheet string) ([][]interface{}, error) {
    resp, err := c.srv.Spreadsheets.Values.Get(c.spreadsheetID, sheet+"!A:Z").Do()
    if err != nil {
        return nil, err
    }
    return resp.Values, nil
}

func (c *Client) appendRow(sheet string, row []interface{}) error {
    vr := &sheetsv4.ValueRange{Values: [][]interface{}{row}}
    _, err := c.srv.Spreadsheets.Values.Append(c.spreadsheetID, sheet+"!A:Z", vr).
        ValueInputOption("RAW").
        InsertDataOption("INSERT_ROWS").
        Do()
    return err
}

func (c *Client) updateCell(sheet, a1 string, value interface{}) error {
    vr := &sheetsv4.ValueRange{Values: [][]interface{}{{value}}}
    _, err := c.srv.Spreadsheets.Values.Update(c.spreadsheetID, sheet+"!"+a1, vr).
        ValueInputOption("RAW").
        Do()
    return err
}

// ---------- Participants ----------

func (c *Client) GetParticipant(tgID int64) (*models.Participant, int, error) {
    values, err := c.readAll(SheetParticipants)
    if err != nil {
        return nil, 0, err
    }
    // header row at index 0
    for i := 1; i < len(values); i++ {
        row := values[i]
        if len(row) < 1 {
            continue
        }
        if fmt.Sprint(row[0]) == strconv.FormatInt(tgID, 10) {
            p := &models.Participant{
                TgID:      tgID,
                FirstName: get(row, 1),
                LastName:  get(row, 2),
                Nick:      get(row, 3),
                TeamName:  get(row, 4),
                CreatedAt: get(row, 5),
            }
            return p, i + 1, nil // sheet rows are 1-indexed; i is 0-indexed in values
        }
    }
    return nil, 0, nil
}

func (c *Client) CreateParticipant(p models.Participant) error {
    return c.appendRow(SheetParticipants, []interface{}{
        p.TgID, p.FirstName, p.LastName, p.Nick, p.TeamName, p.CreatedAt,
    })
}

func (c *Client) UpdateParticipantTeam(tgID int64, teamName string) error {
    _, rowNum, err := c.GetParticipant(tgID)
    if err != nil {
        return err
    }
    if rowNum == 0 {
        return fmt.Errorf("participant not found")
    }
    // column E = 5th column
    a1 := fmt.Sprintf("E%d", rowNum)
    return c.updateCell(SheetParticipants, a1, teamName)
}

// ---------- Teams ----------

func (c *Client) ListTeams() ([]models.Team, error) {
    values, err := c.readAll(SheetTeams)
    if err != nil {
        return nil, err
    }
    teams := []models.Team{}
    for i := 1; i < len(values); i++ {
        row := values[i]
        if len(row) == 0 {
            continue
        }
        t := models.Team{
            TeamID:    get(row, 0),
            TeamName:  get(row, 1),
            CreatedAt: get(row, 2),
        }
        if strings.TrimSpace(t.TeamName) != "" {
            teams = append(teams, t)
        }
    }
    return teams, nil
}

func (c *Client) CreateTeam(name string) (models.Team, error) {
    name = strings.TrimSpace(name)
    if name == "" {
        return models.Team{}, fmt.Errorf("team name empty")
    }
    // generate team_id: short slug
    id := slug(name)
    t := models.Team{TeamID: id, TeamName: name, CreatedAt: util.NowISO()}
    if err := c.appendRow(SheetTeams, []interface{}{t.TeamID, t.TeamName, t.CreatedAt}); err != nil {
        return models.Team{}, err
    }
    return t, nil
}

func slug(s string) string {
    s = strings.ToLower(strings.TrimSpace(s))
    b := strings.Builder{}
    for _, r := range s {
        if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
            b.WriteRune(r)
        } else if r == ' ' || r == '-' || r == '_' {
            b.WriteRune('-')
        }
    }
    out := strings.Trim(b.String(), "-")
    if out == "" {
        out = "team"
    }
    return out
}

// ---------- Stages ----------

func (c *Client) ListStages(all bool) ([]models.Stage, error) {
    values, err := c.readAll(SheetStages)
    if err != nil {
        return nil, err
    }
    stages := []models.Stage{}
    for i := 1; i < len(values); i++ {
        row := values[i]
        if len(row) == 0 {
            continue
        }
        st := models.Stage{
            StageID: get(row, 0),
            Title:   get(row, 1),
            Date:    get(row, 2),
            Time:    get(row, 3),
            Place:   get(row, 4),
            Address: get(row, 5),
            RegOpen: get(row, 6),
            Price:   get(row, 7),
        }
        if strings.TrimSpace(st.StageID) == "" || strings.TrimSpace(st.Title) == "" {
            continue
        }
        if !all && !util.NormalizeBoolRU(st.RegOpen) {
            continue
        }
        stages = append(stages, st)
    }
    return stages, nil
}

func (c *Client) GetStage(stageID string) (*models.Stage, error) {
    stages, err := c.ListStages(true)
    if err != nil {
        return nil, err
    }
    for _, s := range stages {
        if s.StageID == stageID {
            ss := s
            return &ss, nil
        }
    }
    return nil, nil
}

func (c *Client) CreateStage(s models.Stage) error {
    return c.appendRow(SheetStages, []interface{}{
        s.StageID, s.Title, s.Date, s.Time, s.Place, s.Address, s.RegOpen, s.Price,
    })
}

func (c *Client) SetStageRegOpen(stageID string, open bool) error {
    values, err := c.readAll(SheetStages)
    if err != nil {
        return err
    }
    for i := 1; i < len(values); i++ {
        row := values[i]
        if get(row, 0) == stageID {
            rowNum := i + 1
            a1 := fmt.Sprintf("G%d", rowNum) // reg_open column
            if open {
                return c.updateCell(SheetStages, a1, "да")
            }
            return c.updateCell(SheetStages, a1, "нет")
        }
    }
    return fmt.Errorf("stage not found")
}

// ---------- Registrations ----------

func (c *Client) ListRegistrationsForStage(stageID string) ([]models.Registration, error) {
    values, err := c.readAll(SheetRegistrations)
    if err != nil {
        return nil, err
    }
    regs := []models.Registration{}
    for i := 1; i < len(values); i++ {
        row := values[i]
        if len(row) == 0 {
            continue
        }
        if get(row, 0) != stageID {
            continue
        }
        tgID, _ := strconv.ParseInt(get(row, 1), 10, 64)
        regs = append(regs, models.Registration{
            StageID:   stageID,
            TgID:      tgID,
            TeamName:  get(row, 2),
            Role:      get(row, 3),
            PayStatus: get(row, 4),
            CreatedAt: get(row, 5),
        })
    }
    return regs, nil
}

func (c *Client) HasRegistration(stageID string, tgID int64) (bool, error) {
    values, err := c.readAll(SheetRegistrations)
    if err != nil {
        return false, err
    }
    tg := strconv.FormatInt(tgID, 10)
    for i := 1; i < len(values); i++ {
        row := values[i]
        if get(row, 0) == stageID && get(row, 1) == tg {
            return true, nil
        }
    }
    return false, nil
}

func (c *Client) CreateRegistration(r models.Registration) error {
    return c.appendRow(SheetRegistrations, []interface{}{
        r.StageID, r.TgID, r.TeamName, r.Role, r.PayStatus, r.CreatedAt,
    })
}

func (c *Client) UpdatePayStatus(stageID string, tgID int64, payStatus string) error {
    values, err := c.readAll(SheetRegistrations)
    if err != nil {
        return err
    }
    tg := strconv.FormatInt(tgID, 10)
    for i := 1; i < len(values); i++ {
        row := values[i]
        if get(row, 0) == stageID && get(row, 1) == tg {
            rowNum := i + 1
            a1 := fmt.Sprintf("E%d", rowNum) // pay_status
            return c.updateCell(SheetRegistrations, a1, payStatus)
        }
    }
    return fmt.Errorf("registration not found")
}

func (c *Client) UpdateRole(stageID string, tgID int64, role string) error {
    values, err := c.readAll(SheetRegistrations)
    if err != nil {
        return err
    }
    tg := strconv.FormatInt(tgID, 10)
    for i := 1; i < len(values); i++ {
        row := values[i]
        if get(row, 0) == stageID && get(row, 1) == tg {
            rowNum := i + 1
            a1 := fmt.Sprintf("D%d", rowNum) // role
            return c.updateCell(SheetRegistrations, a1, role)
        }
    }
    return fmt.Errorf("registration not found")
}

// Count main pilots for team on stage
func (c *Client) CountMainForTeam(stageID, teamName string) (int, error) {
    regs, err := c.ListRegistrationsForStage(stageID)
    if err != nil {
        return 0, err
    }
    cnt := 0
    for _, r := range regs {
        if strings.EqualFold(strings.TrimSpace(r.TeamName), strings.TrimSpace(teamName)) && r.Role == "main" {
            cnt++
        }
    }
    return cnt, nil
}

// ---------- Results & Photos ----------

func (c *Client) GetResult(stageID string, tgID int64) (*models.Result, error) {
    values, err := c.readAll(SheetResults)
    if err != nil {
        return nil, err
    }
    tg := strconv.FormatInt(tgID, 10)
    for i := 1; i < len(values); i++ {
        row := values[i]
        if get(row, 0) == stageID && get(row, 1) == tg {
            return &models.Result{
                StageID:   stageID,
                TgID:      tgID,
                BestTime:  get(row, 2),
                Position:  get(row, 3),
                Points:    get(row, 4),
            }, nil
        }
    }
    return nil, nil
}

func (c *Client) SumPointsForUser(tgID int64) (int, error) {
    values, err := c.readAll(SheetResults)
    if err != nil {
        return 0, err
    }
    tg := strconv.FormatInt(tgID, 10)
    sum := 0
    for i := 1; i < len(values); i++ {
        row := values[i]
        if get(row, 1) == tg {
            p, _ := strconv.Atoi(strings.TrimSpace(get(row, 4)))
            sum += p
        }
    }
    return sum, nil
}

func (c *Client) GetPhoto(stageID string) (*models.Photo, error) {
    values, err := c.readAll(SheetPhotos)
    if err != nil {
        return nil, err
    }
    for i := 1; i < len(values); i++ {
        row := values[i]
        if get(row, 0) == stageID {
            return &models.Photo{StageID: stageID, URL: get(row, 1)}, nil
        }
    }
    return nil, nil
}

// ---------- helpers ----------

func get(row []interface{}, idx int) string {
    if idx < 0 || idx >= len(row) || row[idx] == nil {
        return ""
    }
    return fmt.Sprint(row[idx])
}

func (c *Client) EnsureHeaders() error {
    // optional helper: do nothing (headers are created manually). Kept for extension.
    _ = context.Background()
    return nil
}
