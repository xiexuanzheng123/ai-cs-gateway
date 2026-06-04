package chat

import (
	"context"
	"log"
	"time"
)

const (
	traceLogQueueSize = 200
	traceLogTimeout   = 2 * time.Second
)

type TraceLogger struct {
	store Store
	queue chan TraceLogRecord
}

func NewTraceLogger(store Store) *TraceLogger {
	logger := &TraceLogger{
		store: store,
		queue: make(chan TraceLogRecord, traceLogQueueSize),
	}
	go logger.run()
	return logger
}

func (l *TraceLogger) Enqueue(record TraceLogRecord) {
	if l == nil {
		return
	}
	// 入队前复制 slice，避免 Send 返回前后继续修改同一份底层数组。
	record.Stages = append([]TraceStageRecord(nil), record.Stages...)
	record.RAGMatches = append([]RAGSearchResult(nil), record.RAGMatches...)

	select {
	case l.queue <- record:
	default:
		// 链路日志是观测数据，队列满时宁可丢日志，也不能阻塞用户回复。
		log.Printf("trace log queue full, dropped trace_id=%s", record.TraceID)
	}
}

func (l *TraceLogger) run() {
	for record := range l.queue {
		ctx, cancel := context.WithTimeout(context.Background(), traceLogTimeout)
		if err := l.store.SaveTraceLog(ctx, record); err != nil {
			log.Printf("save trace log failed trace_id=%s error=%v", record.TraceID, err)
		}
		cancel()
	}
}
