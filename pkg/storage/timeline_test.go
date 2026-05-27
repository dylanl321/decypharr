package storage

import (
	"testing"
	"time"
)

func TestAppendFileEvent_PerFileDedupe(t *testing.T) {
	e := &Entry{Timeline: nil}
	e.AppendFileEvent(TimelineFileDownloadStart, "torbox", "a.mkv", "", 0, 0)
	e.AppendFileEvent(TimelineFileDownloadStart, "torbox", "a.mkv", "", 0, 0)
	e.AppendFileEvent(TimelineFileDownloadStart, "torbox", "b.mkv", "", 0, 0)
	if len(e.Timeline) != 2 {
		t.Fatalf("expected 2 events, got %d", len(e.Timeline))
	}
	if e.Timeline[0].File != "a.mkv" || e.Timeline[1].File != "b.mkv" {
		t.Fatalf("unexpected files: %+v", e.Timeline)
	}
}

func TestHasPartialFileFailures(t *testing.T) {
	e := &Entry{}
	e.AppendFileEvent(TimelineFileDownloadComplete, "torbox", "a.mkv", "", 0, 0)
	e.AppendFileEvent(TimelineFileDownloadFailed, "torbox", "b.mkv", "502", 0, 0)
	if !e.HasPartialFileFailures() {
		t.Fatal("expected partial failures")
	}
	retry := e.FilesForLocalRetry([]*File{{Name: "a.mkv"}, {Name: "b.mkv"}})
	if len(retry) != 1 || retry[0].Name != "b.mkv" {
		t.Fatalf("expected only b.mkv to retry, got %+v", retry)
	}
}

func TestAppendFileEvent_RecordsBytesAndDuration(t *testing.T) {
	e := &Entry{}
	dur := 2 * time.Second
	e.AppendFileEvent(TimelineFileDownloadComplete, "torbox", "movie.mkv", "", 1_000_000, dur)
	ev := e.Timeline[0]
	if ev.Bytes != 1_000_000 || ev.Duration != int64(dur) {
		t.Fatalf("bytes/duration: got %d / %d", ev.Bytes, ev.Duration)
	}
}
