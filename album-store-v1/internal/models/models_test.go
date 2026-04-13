package models

import (
	"encoding/json"
	"testing"
)

func TestPhotoStatusResponseOmitsURLUntilCompleted(t *testing.T) {
	tests := []struct {
		name   string
		status string
	}{
		{name: "processing", status: PhotoStatusProcessing},
		{name: "failed", status: PhotoStatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			photo := Photo{
				PhotoID: "photo-1",
				AlbumID: "album-1",
				Seq:     3,
				Status:  tt.status,
				URL:     "https://example.com/photo.jpg",
			}

			response := photo.StatusResponse()

			if response.URL != "" {
				t.Fatalf("expected %s response to omit URL value, got %q", tt.status, response.URL)
			}

			body, err := json.Marshal(response)
			if err != nil {
				t.Fatalf("marshal response: %v", err)
			}
			if string(body) != `{"photo_id":"photo-1","album_id":"album-1","seq":3,"status":"`+tt.status+`"}` {
				t.Fatalf("expected URL field to be absent from JSON, got %s", body)
			}
		})
	}
}

func TestPhotoStatusResponseIncludesURLWhenCompleted(t *testing.T) {
	photo := Photo{
		PhotoID: "photo-1",
		AlbumID: "album-1",
		Seq:     3,
		Status:  PhotoStatusCompleted,
		URL:     "https://example.com/photo.jpg",
	}

	response := photo.StatusResponse()

	if response.URL != photo.URL {
		t.Fatalf("expected completed response URL %q, got %q", photo.URL, response.URL)
	}

	body, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if string(body) != `{"photo_id":"photo-1","album_id":"album-1","seq":3,"status":"completed","url":"https://example.com/photo.jpg"}` {
		t.Fatalf("expected completed JSON to include URL, got %s", body)
	}
}
