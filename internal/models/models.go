package models

type Participant struct {
    TgID      int64
    FirstName string
    LastName  string
    Nick      string
    TeamName  string
    CreatedAt string
}

type Team struct {
    TeamID    string
    TeamName  string
    CreatedAt string
}

type Stage struct {
    StageID  string
    Title    string
    Date     string
    Time     string
    Place    string
    Address  string
    RegOpen  string // "да"/"нет" or "true"/"false" (we normalize)
    Price    string
}

type Registration struct {
    StageID   string
    TgID      int64
    TeamName  string
    Role      string // main/reserve
    PayStatus string // unpaid/paid/cancelled
    CreatedAt string
}

type Result struct {
    StageID   string
    TgID      int64
    BestTime  string
    Position  string
    Points    string
}

type Photo struct {
    StageID string
    URL     string
}
