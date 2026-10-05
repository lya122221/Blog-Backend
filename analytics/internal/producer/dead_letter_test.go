package producer

import (
	"errors"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestDeadLetterRecordPreservesSourceAndReason(t *testing.T) {
	source := &kgo.Record{
		Topic: "article-events", Partition: 2, Offset: 42,
		Key: []byte("article"), Value: []byte("{broken"),
	}
	record := deadLetterRecord("article-events.dlq", source, errors.New("invalid JSON"))
	if record.Topic != "article-events.dlq" || string(record.Key) != "article" || string(record.Value) != "{broken" {
		t.Fatalf("DLQ record = %+v", record)
	}
	want := map[string]string{
		"source_topic": "article-events", "source_partition": "2",
		"source_offset": "42", "error": "invalid JSON",
	}
	if len(record.Headers) != len(want) {
		t.Fatalf("headers = %+v", record.Headers)
	}
	for _, header := range record.Headers {
		if string(header.Value) != want[header.Key] {
			t.Fatalf("header %s = %q, want %q", header.Key, header.Value, want[header.Key])
		}
	}
	source.Value[0] = 'x'
	if string(record.Value) != "{broken" {
		t.Fatalf("DLQ record changed with source: %q", record.Value)
	}
}
