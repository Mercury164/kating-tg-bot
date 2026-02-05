package sheets

import (
    "context"
    "fmt"
    "os"

    "google.golang.org/api/option"
    sheetsv4 "google.golang.org/api/sheets/v4"
)

type Client struct {
    srv *sheetsv4.Service
    spreadsheetID string
}

func New(serviceAccountJSONPath, spreadsheetID string) (*Client, error) {
    if _, err := os.Stat(serviceAccountJSONPath); err != nil {
        return nil, fmt.Errorf("service account json: %w", err)
    }
    ctx := context.Background()
    srv, err := sheetsv4.NewService(ctx,
        option.WithCredentialsFile(serviceAccountJSONPath),
        option.WithScopes(sheetsv4.SpreadsheetsScope),
    )
    if err != nil {
        return nil, err
    }
    return &Client{srv: srv, spreadsheetID: spreadsheetID}, nil
}

func (c *Client) SpreadsheetID() string { return c.spreadsheetID }
