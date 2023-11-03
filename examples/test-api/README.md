test-api input files
====================

The files in this directory are the input to test-api.

* data.json is used to populate the database. The remote information must be
  set to something valid, as it will be passed to the agent to publish metrics
  and logs. Note the agent will need the token listed here.

* test-api.env contains the environment passed to the agent. The example file
  contains the token (as listed in data.json) that the agent will use to
  connect to test-api.
