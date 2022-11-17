# ECS Toolkit

## Configuration

### AWS Credentials

The AWS SDK used internally uses its default credential chain to find AWS
credentials. The SDK detects and uses the built-in providers automatically,
without requiring manual configuration. For example, if you use IAM roles for
Amazon EC2 instances, it automatically use the instance’s credentials. For more
information on setting up AWS credentials, see "[Configuring the AWS
CLI][aws-configuration-cli]" and "[Configuring the AWS SDK for Go
V2][aws-configuration-sdk]".

### Config File

The config file for an application would look like what you see below:

```yaml
---
version: v1
cluster: <name of cluster>
tasks:
  - name: <task-definition-1-name>
    containers:
      - <container-1-name>
  - name: <task-definition-2-name>
    containers:
      - <container-1-name>
      - <container-2-name>
      # add as many containers as needed
  # add as many tasks as needed
services:
  - name: <service-1-name>
    containers:
      - <container-1-name>
  - name: <service-2-name>
    containers:
      - <container-1-name>
  - name: <service-3-name>
    containers:
      - <container-1-name>
  # add as many services as needed
```

## Usage

By default, the tool will use the config file located at `.ecs-toolkit.yml` but
you can specify an alternative using the `--config` flag. With that, it's easy
to deploy your application using the `deploy` command. For example:

```console
$ ecs-toolkit -l debug deploy --image-tag=49779134ca1dcef21f0b5123d3d5c2f4f47da650
2022-11-17T18:48:15+02:00 DBG log level set to debug
2022-11-17T18:48:15+02:00 INF using config file: .ecs-toolkit.yml
2022-11-17T18:48:15+02:00 INF reading .ecs-toolkit.yml config file
2022-11-17T18:48:15+02:00 INF parsing config file
2022-11-17T18:48:15+02:00 INF validating config file
2022-11-17T18:48:15+02:00 INF starting deployment of services cluster=shipatlas
2022-11-17T18:48:15+02:00 INF fetching service profiles: hub-web-server, hub-worker, hub-worker-scheduler cluster=shipatlas
2022-11-17T18:48:17+02:00 WRN some services missing, limiting to: hub-web-server cluster=shipatlas
2022-11-17T18:48:17+02:00 INF fetching service task definition profile cluster=shipatlas service=hub-web-server
2022-11-17T18:48:17+02:00 INF building new task definition from hub-web-server:98 cluster=shipatlas service=hub-web-server
2022-11-17T18:48:17+02:00 DBG container image registry: 995655197720.dkr.ecr.us-east-1.amazonaws.com cluster=shipatlas container=rails service=hub-web-server
2022-11-17T18:48:17+02:00 DBG container image name: shipatlas/hub cluster=shipatlas container=rails service=hub-web-server
2022-11-17T18:48:17+02:00 INF old container image tag: 49779134ca1dcef21f0b5123d3d5c2f4f47da650 cluster=shipatlas container=rails service=hub-web-server
2022-11-17T18:48:17+02:00 INF new container image tag: 49779134ca1dcef21f0b5123d3d5c2f4f47da650 cluster=shipatlas container=rails service=hub-web-server
2022-11-17T18:48:17+02:00 WRN skipping container image tag update, no changes cluster=shipatlas container=rails service=hub-web-server
2022-11-17T18:48:17+02:00 WRN skipping registering new task definition, no changes cluster=shipatlas service=hub-web-server
2022-11-17T18:48:17+02:00 WRN skipping service update, no changes cluster=shipatlas service=hub-web-server
2022-11-17T18:48:17+02:00 INF checking if all services are stable cluster=shipatlas
SDK 2022/11/17 18:48:17 DEBUG attempting waiter request, attempt count: 1
2022-11-17T18:48:17+02:00 INF completed deployment of services cluster=shipatlas
```

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

## License

[ShipAtlas][shipatlas] © 2022. The [MIT License bundled therein][license] is a
permissive license that is short and to the point. It lets people do anything
they want as long as they provide attribution and waive liability.

[aws-configuration-cli]: https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-quickstart.html
[aws-configuration-sdk]: https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/
[golang-quickstart]: https://go.dev/doc/tutorial/getting-started
[golangci-lint-install]: https://golangci-lint.run/usage/install/
[license]: https://raw.githubusercontent.com/shipatlas/ecs-toolkit/main/LICENSE
[shipatlas]: https://www.shipatlas.dev
