# Contributing to Workload-Variant-Autoscaler

Welcome! We're excited that you're interested in contributing to the Workload-Variant-Autoscaler (WVA) project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Submitting Pull Requests](#submitting-pull-requests)
- [Coding Guidelines](#coding-guidelines)
- [Testing](#testing)
- [Documentation](#documentation)
- [Community](#community)

## Code of Conduct

This project follows the [Kubernetes Community Code of Conduct](https://github.com/kubernetes/community/blob/master/code-of-conduct.md). By participating, you are expected to uphold this code.

## Getting Started

### Prerequisites

- Go 1.23.0+
- Docker 17.03+
- kubectl 1.32.0+
- Kind (for local development)
- Basic understanding of Kubernetes controllers

### Setting Up Your Development Environment

1. **Fork the repository** on GitHub

2. **Clone your fork:**
   ```bash
   git clone https://github.com/<your-username>/workload-variant-autoscaler.git
   cd workload-variant-autoscaler
   ```

3. **Add upstream remote:**
   ```bash
   git remote add upstream https://github.com/llm-d-incubation/workload-variant-autoscaler.git
   ```

4. **Install dependencies:**
   ```bash
   go mod download
   ```

5. **Set up a local Kind cluster:**
   ```bash
   make create-kind-cluster
   ```

6. **Run tests to verify setup:**
   ```bash
   make test
   ```

## Development Workflow

### Creating a Feature Branch

Always work on a feature branch:

```bash
git checkout -b feature/my-new-feature
```

Use descriptive branch names:
- `feature/add-new-metric`
- `fix/memory-leak-collector`
- `docs/update-installation-guide`

### Building and Running Locally

**Build the controller:**
```bash
make build
```

**Run the controller locally (outside cluster):**
```bash
make run
```

**Build and push Docker image:**
```bash
make docker-build docker-push IMG=<your-registry>/wva-controller:tag
```

**Deploy to your cluster:**
```bash
make deploy IMG=<your-registry>/wva-controller:tag
```

### Testing Your Changes

**Run unit tests:**
```bash
make test
```

**Run E2E tests:**
```bash
make test-e2e
```

**Run specific E2E tests:**
```bash
make test-e2e FOCUS="single VA"
```

**Run linter:**
```bash
make lint
```

**Auto-fix linting issues:**
```bash
make lint-fix
```

### Making Changes

1. **Keep changes focused:** One logical change per PR
2. **Write tests:** Add unit tests for new functionality
3. **Update documentation:** Keep docs in sync with code changes
4. **Follow conventions:** Match existing code style and patterns

## Submitting Pull Requests

### Before Submitting

- [ ] Run `make test` and ensure all tests pass
- [ ] Run `make lint` and fix any issues
- [ ] Update documentation if needed
- [ ] Add or update tests for your changes
- [ ] Commit messages follow [conventional commits](https://www.conventionalcommits.org/)

### Commit Message Format

Use clear, descriptive commit messages:

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `test`: Adding or updating tests
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `chore`: Maintenance tasks

**Examples:**
```
feat(optimizer): add support for multi-GPU allocation

Implement greedy algorithm for optimal GPU type selection
across multiple models with different service classes.

Closes #123
```

```
fix(collector): resolve memory leak in metrics collection

The collector was not properly releasing Prometheus client
connections, causing gradual memory growth.

Fixes #456
```

### Pull Request Process

1. **Update your fork:**
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Push to your fork:**
   ```bash
   git push origin feature/my-new-feature
   ```

3. **Create a Pull Request** on GitHub

4. **Fill out the PR template** completely

5. **Address review feedback** promptly

6. **Keep PR updated** with main branch:
   ```bash
   git fetch upstream
   git rebase upstream/main
   git push --force-with-lease origin feature/my-new-feature
   ```

### PR Requirements

- All CI checks must pass
- At least one approval from a maintainer
- No unresolved conversations
- Documentation updated if applicable
- Tests added/updated as needed

## Coding Guidelines

### Go Code Style

- Follow standard Go conventions: [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` for formatting (run `make fmt`)
- Follow [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)

### Project Structure

```
workload-variant-autoscaler/
├── api/              # CRD definitions
├── cmd/              # Main applications
├── config/           # Kubernetes manifests
├── deploy/           # Deployment scripts and examples
├── docs/             # Documentation
├── internal/         # Private application code
├── pkg/              # Public libraries
├── test/             # Tests
└── tools/            # Development tools
```

**Import Guidelines:**
- Group imports: stdlib, external, internal
- Use meaningful package names
- Avoid circular dependencies

### Error Handling

- Always check errors
- Wrap errors with context: `fmt.Errorf("context: %w", err)`
- Use structured logging with appropriate levels

**Example:**
```go
if err := doSomething(); err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}
```

### Logging

Use the project's logger from `internal/logger`:

```go
logger.Info("Reconciling VariantAutoscaling", "name", va.Name, "namespace", va.Namespace)
logger.Debug("Computed allocation", "replicas", replicas, "accelerator", accelerator)
logger.Error(err, "Failed to update status")
```

## Testing

### Unit Tests

- Test files should be `*_test.go`
- Use table-driven tests for multiple scenarios
- Mock external dependencies
- Aim for >80% code coverage

**Example:**
```go
func TestOptimizer_Optimize(t *testing.T) {
    tests := []struct {
        name    string
        input   OptimizerInput
        want    OptimizerOutput
        wantErr bool
    }{
        // test cases
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}
```

### E2E Tests

Located in `test/e2e/`:
- Test full controller workflows
- Use real Kubernetes clusters (Kind)
- Clean up resources after tests

### Integration Tests

- Test interactions between components
- Use envtest for Kubernetes API server
- Located in `test/integration/`

## Documentation

### Where to Document

- **User-facing docs:** `docs/user-guide/`
- **Developer docs:** `docs/developer-guide/`
- **Design decisions:** `docs/design/`
- **Integration guides:** `docs/integrations/`
- **Tutorials:** `docs/tutorials/`

### Documentation Standards

- Use clear, concise language
- Include code examples
- Keep docs in sync with code
- Add diagrams for complex concepts
- Test all commands/examples

### Updating CRD Documentation

After modifying CRDs:
```bash
make crd-docs
```

## Community

### Communication Channels

- **GitHub Issues:** Bug reports and feature requests
- **GitHub Discussions:** Questions and community help
- **Community Meetings:** Join llm-d autoscaling meetings

### Getting Help

- Check existing [documentation](docs/)
- Search [GitHub issues](https://github.com/llm-d-incubation/workload-variant-autoscaler/issues)
- Ask in GitHub Discussions
- Attend community meetings

### Reporting Bugs

Use the bug report template when creating issues. Include:
- Clear description of the problem
- Steps to reproduce
- Expected vs actual behavior
- Environment details (K8s version, WVA version, etc.)
- Relevant logs

### Requesting Features

Use the feature request template. Include:
- Clear use case description
- Expected behavior
- Alternative solutions considered
- Willingness to contribute

## Review Process

### What Reviewers Look For

- **Correctness:** Does the code do what it claims?
- **Testing:** Are there adequate tests?
- **Documentation:** Is it documented appropriately?
- **Style:** Does it follow project conventions?
- **Design:** Is it well-architected?

### Response Time

- Initial response: Within 3-5 business days
- Follow-up reviews: Within 2-3 business days
- Stale PRs (no activity >30 days) may be closed

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.

## Recognition

Contributors are recognized in:
- Release notes
- Project README
- Special recognition for significant contributions

---

Thank you for contributing to Workload-Variant-Autoscaler!

