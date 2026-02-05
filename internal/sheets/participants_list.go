package sheets

import (
    "fmt"
    "strconv"
)

func (c *Client) ListParticipantIDs() ([]int64, error) {
    values, err := c.readAll(SheetParticipants)
    if err != nil {
        return nil, err
    }
    out := []int64{}
    for i := 1; i < len(values); i++ {
        row := values[i]
        if len(row) < 1 {
            continue
        }
        idStr := fmt.Sprint(row[0])
        id, err := strconv.ParseInt(idStr, 10, 64)
        if err != nil {
            continue
        }
        out = append(out, id)
    }
    return out, nil
}
