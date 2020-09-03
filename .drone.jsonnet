local step(name, commands, image='golang:1.13.10') = {
  name: name,
  commands: commands,
  image: image,
};


local pipeline(name, steps=[]) = {
  kind: 'pipeline',
  type: 'docker',
  name: name,
  steps: [step('runner identification', ['echo $DRONE_RUNNER_NAME'], 'alpine')] + steps,
};

local masterOnly = {
  when: {branch:['drone']},
};

local repo = 'grafana/synthetic-monitoring-agent';

[
  pipeline('build', [
    step('lint', ['make lint']),
    step('test', ['make test']),
    step('build', [
      'git fetch origin --tags',
      './scripts/version',
      './scripts/version > .tags', // save version in special file for docker plugin
      'make build',
    ]),
    // We can't use 'make docker' without making this repo priveleged in drone
    // so we will use the native docker plugin instead for security.
    step('docker build',[],'plugins/docker')+{
      settings:{
        repo: repo,
        dry_run: 'true',
      }
    },
    step('docker push',[],'plugins/docker')
    + {
        settings:{
          repo: repo,
        }
    }
    + masterOnly,
  ])
]