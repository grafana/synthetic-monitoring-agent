This action will restore Go cache files. The cache key is based on the contents
of the go.sum file, plus the runner's operating system.

This assumes that go is already available to the action.

Note that this should be preceded by a step that checks out the repository, e.g.:

      - name: checkout
        uses: actions/checkout@v4
