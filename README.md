# ECS Toolkit

ECS Toolkit is a convenience tool that aims to make it easier to manage an
application already running on ECS where an application is a group of different ECS services and tasks.

## Introduction

### Use Cases

For example, you can use it within your CI/CD pipeline to rollout new changes to
your running services or even run new tasks with those new changes. While
managing deployments is an obvious use case, the intention is that it will grow
to have more functionality where it makes sense to.

If there's a use-case that you would like considered, [please file an
issue][new-issue] with your request.

### Motivation

Simply put, a lot of the existing tooling doesn't do a good job simplifying the
update process. You kinda have to jump through hoops to make it work.

For example, when updating application configuration on ECS, at some point
you'll need to update the task definition before updating the service or running
a new task. Doing so in most cases involves using the [AWS CLI][aws-cli] to
register a new task definition.

Unfortunately using `register-task-definition` requires the task definition
passed in as JSON. You can get that from `describe-task-definition` but it's not
easy use that output as input for `register-task-definition` without hitting
issues such as [this][why-problem-1], [this][why-problem-2] and
[this][why-problem-3].

And granted, there are other official projects such as [ECS CLI][aws-ecs-cli]
and [Fargate CLI][aws-fargate-cli] that try to provide alternative ways of
managing ECS deployments but they both felt unsatisfactory. Presenting another
take at solving the same problem felt like the natural and best way forward.

### Features

Takes inputs as a config file which allows you to:

* Define all the services and tasks that make up an application in a concise
  configuration file that's easy to grok at a glance i.e. `.ecs-toolkit.yml`,
  you could think of this as an `application.yml` file.
* Define all the important configuration options for tasks and services that are
  most applicable for deployments.

As part of the deployment process, it:

* Registers a new task definition with a new image.
* Runs new tasks and update services with the new task definition.
* Runs the tasks and services asynchronously for faster deployments.
* Watches a service until it's stable or a task until it's stopped.
* Provides extensive logging and sufficient reporting throughout the process to
  catch failures and monitor progress.

It can also:

* Update the image of only select containers in the new task definition.
* Run pre-deployment tasks before updating services e.g. database migrations,
  asset syncing.
* Run post-deployment tasks after updating services e.g. cleanup.
* Optionally skip pre-deployment and post-deployment tasks during deployment.
* Perform redeploys using the same image with an option forcing a pull of the
  image.

If there's a feature that you would like considered, [please file an
issue][new-issue] with your request.

## Configuration

### Application Config File

The application config file describes the tasks and services that make up an
application. For example, a config file for a typical Rails application that
consists of a CDN assets sync, database migration, web-server, background worker
and worker scheduler is shown below:

```yaml
---
version: v1
cluster: example
tasks:
  pre:
    - family: app-asset-sync
      count: 1
      containers:
        - rails
      launch_type: fargate
      network_configuration: 
        vpc_configuration:
          assign_public_ip: true
          security_groups:
            - sg-xxxxxxxxxxxxxxxxx
          subnets:
            - subnet-xxxxxxxxxxxxxxxxx
    - family: app-database-migrate
      count: 1
      containers:
        - rails
      launch_type: ec2
    # add as many tasks as needed
services:
  - name: app-web-server
    containers:
      - rails
  - name: app-worker
    containers:
      - resque
  - name: app-worker-scheduler
    containers:
      - resque-scheduler
  # add as many services as needed
```

#### Top-Level Options

```yaml
# Version of the configuration file i.e. `v1`.
# [Required]
version: <string>

# Name of the ECS cluster which is a logical grouping of tasks or services comprising
# of your application.
# [Required]
cluster: <string>

# List of your application's ECS services to manage. See service options.
# [Required]
tasks: <object>

# List of your application's ECS tasks to manage. See task options.
# [Required]
services: array<object>
```

#### Task Options

```yaml
tasks: <object>

  # List of tasks to run before updating services.
  # [Required]
  pre: array<object>

      # The family for the latest `ACTIVE` revision, family and revision (`family:revision`)
      # for a specific revision in the family, or full Amazon Resource Name (ARN) of the
      # task definition to run a task with.
      # [Required]
    - family: <string>

      # The number of instantiations of the specified task to place on your cluster. You
      # can specify a minimum of one up to ten tasks.
      # [Required]
      count: <integer>

      # List of names of the containers in the task's task definition that should have the
      # image tag updated.
      # [Required]
      containers: array<string>

      # The infrastructure to run your standalone task on i.e. `ec2`, `fargate` or `external`.
      # [Optional]
      launch_type: <string>

      # List of capacity provider strategies to use for the task. If specified, `launch_type`
      # must be omitted. May contain a maximum of 6 capacity providers.
      # [Optional]
      capacity_provider_strategies: array<object>

          # The short name of the capacity provider.
          # [Required]
        - capacity_provider: <string>

          # The base value designates how many tasks, at a minimum, to run on the specified
          # capacity provider. Only one capacity provider in a capacity provider strategy
          # can have a base defined. If no value is specified, the default value of 0 is used.
          # [Optional]
          base: <integer>

          # The weight value designates the relative percentage of the total number of tasks
          # launched that should use the specified capacity provider. The weight value is
          # taken into consideration after the base value, if defined, is satisfied. If no
          # weight value is specified, the default value of 0 is used.
          # [Optional]
          weight: <integer>

      # The network configuration for the task.
      # [Optional]
      network_configuration: <object>

        # The VPC subnets and security groups that are associated with a task. All specified
        # subnets and security groups must be from the same VPC.
        # [Required]
        vpc_configuration: <object>

          # Whether the task's elastic network interface receives a public IP address.
          # [Required]
          assign_public_ip: <boolean>

          # The IDs of the subnets associated with the task. There's a limit of 16 subnets
          # that can be specified and all specified subnets must be from the same VPC.
          # [Required]
          security_groups: array<string>

          # The IDs of the security groups associated with the task. There's a limit of 5
          # security groups that can be specified and all specified security groups must
          # be from the same VPC.
          # [Required]
          subnets: array<string>

  # List of tasks to run after updating services. Same as <tasks.pre>.
  # [Required]
  post: array<object>
```

#### Service Options

```yaml
services: array<object>
    # The name of the service.
    # [Required]
  - name: <string>

    # List of names of the containers in the service's task definition that should have the
    # image tag updated.
    # [Required]
    containers: array<string>

    # Determines whether to force a new deployment of the service. By default, deployments
    # aren't forced. You can use this option to start a new deployment with no service
    # definition changes.
    # [Optional]
    force: <boolean>

    # Maximum duration in minutes to wait for the service to be stable. Defaults to 15
    # minutes.
    # [Optional]
    max_wait: <integer>
```

### AWS Credentials

The AWS SDK used internally uses its default credential chain to find AWS
credentials. The SDK detects and uses the built-in providers automatically,
without requiring manual configuration. For example, if you use IAM roles for
Amazon EC2 instances, it automatically use the instance’s credentials.

### IAM Policy

Regardless of the credential setup, the owner of the credentials should have a
policy that assigns the required permissions, which are:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "ecs:DescribeServices",
                "ecs:DescribeTasks",
                "ecs:RunTask",
                "ecs:UpdateService"
            ],
            "Resource": "*",
            "Condition": {
                "ArnEquals": {
                    "ecs:cluster": "arn:aws:ecs:${Region}:${Account}:cluster/${ClusterName}"
                }
            }
        },
        {
            "Effect": "Allow",
            "Action": [
                "ecs:RegisterTaskDefinition",
                "ecs:ListTaskDefinitions",
                "ecs:DescribeTaskDefinition"
            ],
            "Resource": "*"
        },
        {
            "Effect": "Allow",
            "Action": "iam:PassRole",
            "Resource": "*",
            "Condition": {
                "StringEquals": {
                    "iam:PassedToService": "ecs-tasks.amazonaws.com"
                }
            }
        }
    ]
}
```

## Usage

### Deploying

Taking the below definition of an Rails application with a database migration
and a web-server:

```yaml
---
version: v1
cluster: example
tasks:
  pre:
    - family: app-database-migrate
      count: 1
      containers:
        - rails
      launch_type: ec2
services:
  - name: app-web-server
    force: false
    containers:
      - rails
  ```

It is easy to deploy a new version of your application using the `deploy`
command:

```console
$ ecs-toolkit deploy --image-tag=49779134ca1dcef21f0b5123d3d5c2f4f47da650
INFO[0000] using config file: .ecs-toolkit.yml          
INFO[0000] reading .ecs-toolkit.yml config file         
INFO[0000] starting rollout to tasks                     cluster=example
INFO[0001] building new task definition from app-database-migrate:24  cluster=example task=app-database-migrate
WARN[0001] skipping container image tag update, no changes  cluster=example container=rails task=app-database-migrate
WARN[0001] skipping registering new task definition, no changes  cluster=example task=app-database-migrate
INFO[0001] preparing running task parameters             cluster=example task=app-database-migrate
INFO[0001] no changes to previous task definition, using latest  cluster=example task=app-database-migrate
INFO[0001] running new task, desired count: 1            cluster=example task=app-database-migrate
INFO[0001] watching task [1] ... last status: pending, desired status: running, health: unknown  cluster=example task=app-database-migrate task-id=85d76f5ffddf42e2bbeabd787c36f097
INFO[0004] watching task [1] ... last status: running, desired status: running, health: unknown  cluster=example task=app-database-migrate task-id=85d76f5ffddf42e2bbeabd787c36f097
INFO[0007] watching task [1] ... last status: running, desired status: running, health: unknown  cluster=example task=app-database-migrate task-id=85d76f5ffddf42e2bbeabd787c36f097
INFO[0010] watching task [1] ... last status: running, desired status: running, health: unknown  cluster=example task=app-database-migrate task-id=85d76f5ffddf42e2bbeabd787c36f097
INFO[0013] watching task [1] ... last status: stopped, desired status: stopped, health: unknown  cluster=example task=app-database-migrate task-id=85d76f5ffddf42e2bbeabd787c36f097
INFO[0013] successfully stopped task [1], reason: essential container in task exited  cluster=example task=app-database-migrate task-id=85d76f5ffddf42e2bbeabd787c36f097
INFO[0013] tasks ran to completion, desired count: 1     cluster=example task=app-database-migrate
INFO[0013] tasks report - total: 1, successful: 1, failed: 0  cluster=example
INFO[0013] completed rollout to tasks                    cluster=example
INFO[0013] starting rollout to services                  cluster=example
INFO[0014] building new task definition from app-web-server:103  cluster=example service=app-web-server
WARN[0014] skipping container image tag update, no changes  cluster=example container=rails service=app-web-server
WARN[0014] skipping registering new task definition, no changes  cluster=example service=app-web-server
INFO[0014] no changes to previous task definition, using latest  cluster=example service=app-web-server
INFO[0016] updated service successfully                  cluster=example service=app-web-server
INFO[0016] watch service rollout progress                cluster=example service=app-web-server
INFO[0017] watching ... service: active, deployment: primary, rollout: 1/1 (0 pending)  cluster=example deployment-id=ecs-svc/6300252591410027253 service=app-web-server
INFO[0017] checking if service is stable                 cluster=example service=app-web-server
INFO[0018] service is stable                             cluster=example service=app-web-server
INFO[0018] services report - total: 1, successful: 1, failed: 0  cluster=example
INFO[0018] completed rollout to services                 cluster=example
```

For more information see `ecs-toolkit --help` or `ecs-toolkit <command> --help`.

## Development

Below instructions are only necessary if you intend to work on the source code.
For normal usage the above instructions should do.

### Requirements

1. Ensure that you have a [properly configured][golang-quickstart] Go workspace.

### Building

1. Clone the repository.
2. Fetch the dependencies with `go get -v github.com/shipatlas/ecs-toolkit`.
4. Install application dependencies via `make dependencies` (they'll be placed
   in `./vendor`).
5. Build and install the binary with `make build`.
6. Run the command e.g. `./bin/ecs-toolkit help` as a basic test.

### Testing

1. Install the `golangci-lint`, [see instructions here][golangci-lint-install].
2. Run linter using `make lint` and test using `make test`.

## Inspiration

* https://github.com/aws/amazon-ecs-cli
* https://github.com/awslabs/fargatecli
* https://github.com/DispatchBot/node-ecs-deployer
* https://github.com/fabfuel/ecs-deploy
* https://github.com/lumoslabs/broadside
* https://github.com/silinternational/ecs-deploy

## License

[King'ori Maina][kingori] © 2022. The [Apache 2.0 License bundled therein][license]
whose main conditions require preservation of copyright and license notices.
Contributors provide an express grant of patent rights. Licensed works,
modifications, and larger works may be distributed under different terms and
without source code.

[aws-cli]: https://aws.amazon.com/cli/
[aws-ecr-register-tf]: https://docs.aws.amazon.com/cli/latest/reference/ecs/register-task-definition.html
[aws-ecs-cli]: https://github.com/aws/amazon-ecs-cli
[aws-fargate-cli]: https://github.com/awslabs/fargatecli
[golang-quickstart]: https://go.dev/doc/tutorial/getting-started
[golangci-lint-install]: https://golangci-lint.run/usage/install/
[kingori]: https://kingori.co
[license]: https://github.com/shipatlas/ecs-toolkit/blob/main/LICENSE.txt
[new-issue]: https://github.com/shipatlas/ecs-toolkit/issues/new
[why-problem-1]: https://github.com/aws/aws-sdk/issues/406#issuecomment-1314182988
[why-problem-2]: https://github.com/aws/aws-sdk/issues/406#issuecomment-1314183046
[why-problem-3]: https://github.com/aws/aws-sdk/issues/406#issuecomment-1314183514
