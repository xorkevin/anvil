package component

type (
	Fetcher interface {
		Fetch(repo, ref string) error
	}
)
