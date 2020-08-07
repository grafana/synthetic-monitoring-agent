package version

var (
	version    = "unknown"
	commit     = "NA"
	buildstamp string
)

func Short() string {
	return version
}

func Commit() string {
	return commit
}

func Buildstamp() string {
	return buildstamp
}
