package component

import (
	"context"
)

type (
	FetcherMock struct {
	}
)

func NewFetcherMock() *FetcherMock {
	return &FetcherMock{}
}

func (f *FetcherMock) Fetch(ctx context.Context, kind, repo, ref string) error {
	return nil
}
