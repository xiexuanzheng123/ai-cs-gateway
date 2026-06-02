package chat

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMySQLStoreSaveFeedbackInsertsFeedback(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	store := NewMySQLStore(db)
	mock.ExpectExec("INSERT INTO cs_feedback").
		WithArgs(
			"session-test",
			"message-test",
			"user-001",
			"thumbs_down",
			"没有解决",
			"handoff",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = store.SaveFeedback(context.Background(), FeedbackRecord{
		ConversationID: "session-test",
		MessageID:      "message-test",
		UserID:         "user-001",
		Rating:         "thumbs_down",
		Comment:        "没有解决",
		ActionTaken:    "handoff",
	})

	if err != nil {
		t.Fatalf("SaveFeedback returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
