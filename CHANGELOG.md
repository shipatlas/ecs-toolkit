# Changelog

## 0.2.1

* Fix runtime panic when watching task - https://github.com/shipatlas/ecs-toolkit/pull/30.

## 0.2.0

* Remove source modification info from `version` command - https://github.com/shipatlas/ecs-toolkit/pull/28.

## 0.1.0

First prototype of the idea with deploy feature. Takes inputs as a config file
which allows you to:

* Define all the services and tasks that make up an application in a concise
  configuration file that's easy to grok at a glance i.e. `.ecs-toolkit.yml`,
  you could think of this as an `application.yml` file.
* Define all the important configuration options for tasks and services that are
  most applicable for deployments.

As part of the deployment process, it:

* Registers a new task definition with a new image cloning all the concurrent
  configuration from the old one to the new one.
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
