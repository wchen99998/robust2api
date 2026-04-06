package service

import (
	"context"
	"time"
)

// opsRepoMock is a test-only OpsRepository implementation with optional function hooks.
type opsRepoMock struct {
	InsertErrorLogFn       func(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error)
	BatchInsertErrorLogsFn func(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error)
}

func (m *opsRepoMock) InsertErrorLog(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error) {
	if m.InsertErrorLogFn != nil {
		return m.InsertErrorLogFn(ctx, input)
	}
	return 0, nil
}

func (m *opsRepoMock) BatchInsertErrorLogs(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error) {
	if m.BatchInsertErrorLogsFn != nil {
		return m.BatchInsertErrorLogsFn(ctx, inputs)
	}
	return int64(len(inputs)), nil
}

func (m *opsRepoMock) ListErrorLogs(ctx context.Context, filter *OpsErrorLogFilter) (*OpsErrorLogList, error) {
	return &OpsErrorLogList{Errors: []*OpsErrorLog{}, Page: 1, PageSize: 20}, nil
}

func (m *opsRepoMock) GetErrorLogByID(ctx context.Context, id int64) (*OpsErrorLogDetail, error) {
	return &OpsErrorLogDetail{}, nil
}

func (m *opsRepoMock) ListRequestDetails(ctx context.Context, filter *OpsRequestDetailFilter) ([]*OpsRequestDetail, int64, error) {
	return []*OpsRequestDetail{}, 0, nil
}

func (m *opsRepoMock) InsertRetryAttempt(ctx context.Context, input *OpsInsertRetryAttemptInput) (int64, error) {
	return 0, nil
}

func (m *opsRepoMock) UpdateRetryAttempt(ctx context.Context, input *OpsUpdateRetryAttemptInput) error {
	return nil
}

func (m *opsRepoMock) GetLatestRetryAttemptForError(ctx context.Context, sourceErrorID int64) (*OpsRetryAttempt, error) {
	return nil, nil
}

func (m *opsRepoMock) ListRetryAttemptsByErrorID(ctx context.Context, sourceErrorID int64, limit int) ([]*OpsRetryAttempt, error) {
	return []*OpsRetryAttempt{}, nil
}

func (m *opsRepoMock) UpdateErrorResolution(ctx context.Context, errorID int64, resolved bool, resolvedByUserID *int64, resolvedRetryID *int64, resolvedAt *time.Time) error {
	return nil
}

var _ OpsRepository = (*opsRepoMock)(nil)
