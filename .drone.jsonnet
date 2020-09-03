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

[
  pipeline('build', [
    //step('lint', ['make lint']),
    // step('test', ['make test']),
    // step('build', [
    //   'git fetch origin --tags',
    //   './scripts/version',
    //   'make build',
    // ]),
    step('package',[
      'git fetch origin --tags',
      './scripts/version',
      'make docker',
      'make package'
    ],'circleci/golang:1.13.10')
  ])
]