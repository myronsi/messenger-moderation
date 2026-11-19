package review

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

func EncodeCursor(c Cursor) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func DecodeCursor(s string) (Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: unknown cursor", ErrInvalid)
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil || c.UserID <= 0 {
		return Cursor{}, fmt.Errorf("%w: unknown cursor", ErrInvalid)
	}
	return c, nil
}
