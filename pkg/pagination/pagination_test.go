package pagination

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

type record struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func TestNormalizeDefaultsAndCapsLimit(t *testing.T) {
	params, err := Normalize(Params{}, 20, 100)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if params.Limit != 20 {
		t.Errorf("Limit = %d, want default 20", params.Limit)
	}

	params, err = Normalize(Params{Limit: 500}, 20, 100)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if params.Limit != 100 {
		t.Errorf("Limit = %d, want cap 100", params.Limit)
	}
}

func TestNormalizeRejectsInvalidLimitsAndCursor(t *testing.T) {
	tests := []struct {
		name   string
		params Params
		def    int
		max    int
	}{
		{"negative limit", Params{Limit: -1}, 10, 50},
		{"zero default", Params{}, 0, 50},
		{"max below default", Params{}, 20, 10},
		{"malformed cursor", Params{After: "!!!"}, 10, 50},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Normalize(test.params, test.def, test.max); err == nil {
				t.Fatal("Normalize() error = nil, want validation error")
			}
		})
	}
}

func TestBuildPageWithExtraRow(t *testing.T) {
	rows := []record{
		{ID: "a", Name: "Ada"},
		{ID: "b", Name: "Grace"},
		{ID: "c", Name: "Margaret"},
	}
	params := Params{Limit: 2}
	total := 12

	page, err := Build(rows, params, func(item record) string { return item.ID }, &total)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(page.Data) != 2 {
		t.Fatalf("len(Data) = %d, want 2", len(page.Data))
	}
	if page.Meta.Limit != 2 || !page.Meta.HasMore {
		t.Errorf("Meta = %+v, want limit 2 and hasMore true", page.Meta)
	}
	if page.Meta.NextCursor != EncodeCursor("b") {
		t.Errorf("NextCursor = %q, want encoded cursor for last returned item", page.Meta.NextCursor)
	}
	if page.Meta.TotalCount == nil || *page.Meta.TotalCount != total {
		t.Errorf("TotalCount = %v, want %d", page.Meta.TotalCount, total)
	}
}

func TestBuildPageWithoutMoreResults(t *testing.T) {
	rows := []record{{ID: "a"}, {ID: "b"}}

	page, err := Build(rows, Params{Limit: 2}, func(item record) string {
		return item.ID
	}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !reflect.DeepEqual(page.Data, rows) {
		t.Errorf("Data = %#v, want %#v", page.Data, rows)
	}
	if page.Meta.HasMore {
		t.Error("HasMore = true, want false")
	}
	if page.Meta.NextCursor != "" {
		t.Errorf("NextCursor = %q, want empty", page.Meta.NextCursor)
	}
	if page.Meta.TotalCount != nil {
		t.Errorf("TotalCount = %v, want nil when count was not supplied", page.Meta.TotalCount)
	}
}

func TestBuildEmptyPageUsesEmptyArray(t *testing.T) {
	page, err := Build([]record(nil), Params{Limit: 5}, func(item record) string {
		return item.ID
	}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if page.Data == nil {
		t.Fatal("Data is nil, want empty slice")
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("marshal page: %v", err)
	}
	if got, want := string(encoded), `{"data":[],"meta":{"limit":5,"hasMore":false}}`; got != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}

func TestBuildRejectsInvalidInput(t *testing.T) {
	if _, err := Build([]record{{ID: "a"}}, Params{Limit: 0}, func(item record) string { return item.ID }, nil); err == nil {
		t.Error("Build() error = nil for non-positive limit")
	}
	if _, err := Build([]record{{ID: "a"}}, Params{Limit: 1}, nil, nil); err == nil {
		t.Error("Build() error = nil for nil cursor function")
	}
	if _, err := Build([]record{{ID: ""}, {ID: "b"}}, Params{Limit: 1}, func(item record) string { return item.ID }, nil); err == nil {
		t.Error("Build() error = nil for empty cursor on last returned item")
	}
}

func TestCursorRoundTrip(t *testing.T) {
	for _, value := range []string{"record/123", "cursor with spaces", "unicode-å"} {
		cursor := EncodeCursor(value)
		got, err := DecodeCursor(cursor)
		if err != nil {
			t.Fatalf("DecodeCursor(%q) error = %v", cursor, err)
		}
		if got != value {
			t.Errorf("DecodeCursor(EncodeCursor(%q)) = %q", value, got)
		}
	}
}

func TestDecodeCursorRejectsInvalidValues(t *testing.T) {
	for _, cursor := range []string{"", "!!!", EncodeCursor("")} {
		if _, err := DecodeCursor(cursor); err == nil {
			t.Errorf("DecodeCursor(%q) error = nil, want error", cursor)
		}
	}
}

func TestTimeIDCursorValueRoundTrip(t *testing.T) {
	timestamp := time.Date(2026, 10, 6, 17, 30, 45, 123456789, time.FixedZone("test", 3600))
	value := FormatTimeIDCursorValue(timestamp, "item-123")
	gotTimestamp, gotID, err := ParseTimeIDCursorValue(value)
	if err != nil {
		t.Fatalf("ParseTimeIDCursorValue() error = %v", err)
	}
	if !gotTimestamp.Equal(timestamp) {
		t.Errorf("timestamp = %s, want %s", gotTimestamp, timestamp)
	}
	if gotID != "item-123" {
		t.Errorf("ID = %q, want %q", gotID, "item-123")
	}
}

func TestParseTimeIDCursorValueRejectsMalformedValues(t *testing.T) {
	for _, value := range []string{
		"",
		"not-a-timestamp|item-123",
		"2026-10-06T17:30:45Z|",
		"2026-10-06T17:30:45Z",
	} {
		if _, _, err := ParseTimeIDCursorValue(value); err == nil {
			t.Errorf("ParseTimeIDCursorValue(%q) error = nil, want error", value)
		}
	}
}
