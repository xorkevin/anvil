package component

type (
	FetcherMock struct {
	}
)

func NewFetcherMock() *FetcherMock {
	return &FetcherMock{}
}

func (f *FetcherMock) Fetch(kind, repo, ref string) error {
	return nil
}
