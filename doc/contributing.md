## Contributing to Caelus
Welcome to report Issues or send pull requests. It's recommended to read the following Contributing Guide first before
contributing.

## Issues

### Search Known Issues First
Please search the existing issues to see if any similar issue or feature request has already been filed. You should
make sure your issue isn't redundant.

### Reporting New Issues
If you open an issue, the more information the better. Such as detailed description, screenshot or video of your
problem, logcat or code blocks for your crash.

## Pull Requests
We strongly welcome your pull request to make Caelus better.

### Branch Management
There are two main branches here:

- master branch
  
  The developing branch. We welcome bugfix, features, typo and whatever on this branch.

- branch-* branch

  The releasing branch. It is our stable release branch. You are recommended to submit bugfix only to these branches.

### Make Pull Requests
The code team will monitor all pull request. Before submitting a pull request, please make sure the followings are done:

  1. Fork the repo and create your branch from master.
  2. Update code or documentation if you have changed APIs.
  3. Check your code lints and checkstyles.
  4. Test your code.
  5. Submit your pull request to master branch.

## Code Style Guide
We use gofmt to check code styles. Make sure gofmt cmd/... pkg/... contrib/... passes

## License
Caelus is under the Apache License 2.0. See the [License](../LICENSE) file for details.
