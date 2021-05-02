package component

type (
	Fetcher interface {
		Fetch(kind, repo, ref string) error
	}

	OSFetcher struct {
		Base string
	}
)

func NewOSFetcher(base string) *OSFetcher {
	return &OSFetcher{
		Base: base,
	}
}

func (o *OSFetcher) Fetch(kind, repo, ref string) error {
	return nil
}
