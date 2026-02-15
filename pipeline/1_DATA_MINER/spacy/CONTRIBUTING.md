# Contributing to Go-Spacy

We welcome contributions to Go-Spacy! This guide will help you get started with contributing to the project, whether you're fixing bugs, adding features, improving documentation, or helping with testing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Contributing Process](#contributing-process)
- [Code Guidelines](#code-guidelines)
- [Testing](#testing)
- [Documentation](#documentation)
- [Pull Request Process](#pull-request-process)
- [Issue Guidelines](#issue-guidelines)
- [Community](#community)

## Code of Conduct

This project adheres to a code of conduct that we expect all contributors to honor. Please be respectful, inclusive, and considerate in all interactions.

### Our Standards

- Use welcoming and inclusive language
- Be respectful of differing viewpoints and experiences
- Gracefully accept constructive criticism
- Focus on what is best for the community
- Show empathy towards other community members

## Getting Started

### Types of Contributions

We welcome several types of contributions:

- **Bug Fixes**: Help identify and fix issues
- **Feature Additions**: Implement new NLP capabilities
- **Performance Improvements**: Optimize existing code
- **Documentation**: Improve guides, examples, and API docs
- **Testing**: Add test cases and improve test coverage
- **Language Support**: Add support for new Spacy models
- **Examples**: Create tutorials and usage examples

### Where to Start

1. **Good First Issues**: Look for issues labeled `good-first-issue`
2. **Documentation**: Always a great place to start contributing
3. **Testing**: Add tests for existing functionality
4. **Examples**: Create examples for common use cases

## Development Setup

### Prerequisites

- Go 1.16+ installed and configured
- Python 3.9+ with development headers
- C++ compiler (GCC 7+ or Clang 7+)
- Git for version control
- Make for build automation

### Fork and Clone

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/your-username/spacy.git
   cd spacy
   ```
3. **Add upstream remote**:
   ```bash
   git remote add upstream https://github.com/original-repo/spacy.git
   ```

### Environment Setup

1. **Install Python dependencies**:
   ```bash
   pip install spacy
   python -m spacy download en_core_web_sm
   ```

2. **Set up Go environment**:
   ```bash
   export CGO_ENABLED=1
   export GO111MODULE=on
   ```

3. **Build the project**:
   ```bash
   make clean && make
   ```

4. **Run tests to verify setup**:
   ```bash
   go test -v
   ```

### IDE Configuration

#### VS Code
Install recommended extensions:
- Go extension
- C/C++ extension
- Python extension

#### GoLand/IntelliJ
- Enable Go modules support
- Configure CGO support
- Set up Python interpreter

## Contributing Process

### 1. Choose an Issue

- Browse [open issues](https://github.com/yourusername/spacy/issues)
- Comment on the issue to express interest
- Wait for maintainer assignment (for larger features)

### 2. Create a Branch

```bash
# Sync with upstream
git checkout main
git pull upstream main

# Create feature branch
git checkout -b feature/your-feature-name

# Or for bug fixes
git checkout -b fix/issue-description
```

### 3. Make Changes

- Write code following our [Code Guidelines](#code-guidelines)
- Add/update tests as appropriate
- Update documentation if needed
- Run tests locally before committing

### 4. Commit Changes

Use conventional commit messages:

```bash
git add .
git commit -m "feat: add support for custom tokenization rules"

# Or for bug fixes
git commit -m "fix: resolve memory leak in NLP.Close()"

# Or for docs
git commit -m "docs: add advanced usage examples"
```

### 5. Push and Create PR

```bash
git push origin your-branch-name
```

Then create a Pull Request on GitHub.

## Code Guidelines

### Go Code Style

Follow standard Go conventions:

```go
// Good
func (n *NLP) ProcessText(text string) ([]Token, error) {
    if text == "" {
        return nil, fmt.Errorf("empty text provided")
    }

    // Process text...
    tokens := n.tokenize(text)
    return tokens, nil
}

// Document all public functions
// ProcessText analyzes input text and returns tokens with linguistic annotations.
//
// The function handles empty input gracefully and returns an error for invalid
// operations. Use this for basic tokenization needs.
//
// Parameters:
//   - text: Input text to process
//
// Returns:
//   - []Token: Slice of tokens with annotations
//   - error: Error if processing fails
//
// Example:
//   tokens, err := nlp.ProcessText("Hello world")
//   if err != nil {
//       log.Fatal(err)
//   }
func (n *NLP) ProcessText(text string) ([]Token, error) {
    // Implementation...
}
```

### Code Organization

- **Package Structure**: Keep related functionality together
- **Error Handling**: Always handle errors appropriately
- **Memory Management**: Clean up resources properly
- **Thread Safety**: Document thread safety guarantees
- **Performance**: Consider performance implications

### C++ Code Style

For C++ bridge code:

```cpp
// Use RAII for resource management
class SpacyWrapper {
public:
    explicit SpacyWrapper(const std::string& model_name)
        : model_name_(model_name) {}

    ~SpacyWrapper() {
        cleanup();
    }

    // Prevent copying
    SpacyWrapper(const SpacyWrapper&) = delete;
    SpacyWrapper& operator=(const SpacyWrapper&) = delete;

private:
    void cleanup() {
        // Clean up resources
    }

    std::string model_name_;
};
```

### Documentation Standards

- **All public APIs** must have comprehensive godoc comments
- **Examples** should be included for complex functions
- **Error conditions** should be documented
- **Thread safety** should be explicitly mentioned
- **Performance notes** for expensive operations

## Testing

### Test Structure

We use several types of tests:

1. **Unit Tests**: Test individual functions
2. **Integration Tests**: Test full workflows
3. **Benchmark Tests**: Performance testing
4. **Multi-language Tests**: Cross-language functionality

### Writing Tests

```go
func TestTokenizeBasic(t *testing.T) {
    nlp, err := NewNLP("en_core_web_sm")
    if err != nil {
        t.Skip("Model not available")
    }
    defer nlp.Close()

    tokens := nlp.Tokenize("Hello world")

    if len(tokens) != 2 {
        t.Errorf("Expected 2 tokens, got %d", len(tokens))
    }

    if tokens[0].Text != "Hello" {
        t.Errorf("Expected 'Hello', got '%s'", tokens[0].Text)
    }
}

func BenchmarkTokenizeSmallText(b *testing.B) {
    nlp, _ := NewNLP("en_core_web_sm")
    defer nlp.Close()

    text := "The quick brown fox jumps over the lazy dog."

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        nlp.Tokenize(text)
    }
}
```

### Running Tests

```bash
# Run all tests
go test -v

# Run specific tests
go test -v -run TestTokenize

# Run benchmarks
go test -bench=. -benchmem

# Test with race detector
go test -race -v

# Test coverage
go test -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Test Requirements

- **New Features**: Must include comprehensive tests
- **Bug Fixes**: Must include regression tests
- **Performance Changes**: Must include benchmarks
- **Cross-platform**: Consider different OS/architectures

## Documentation

### Types of Documentation

1. **API Documentation**: Godoc comments for all public APIs
2. **User Guides**: README, installation, tutorials
3. **Developer Docs**: Architecture, contributing guides
4. **Examples**: Practical usage examples

### Documentation Standards

- **Clear and Concise**: Easy to understand
- **Complete**: Cover all parameters and return values
- **Examples**: Show real usage patterns
- **Up-to-date**: Keep in sync with code changes

### Building Documentation

```bash
# Generate godoc
godoc -http=:6060
# Visit http://localhost:6060/pkg/your-package/

# Or use go doc
go doc -all
```

## Pull Request Process

### Before Submitting

- [ ] Tests pass locally (`go test -v`)
- [ ] Code follows style guidelines
- [ ] Documentation is updated
- [ ] Commit messages are clear
- [ ] Branch is up-to-date with main

### PR Description Template

```markdown
## Description
Brief description of changes made.

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update
- [ ] Performance improvement

## Testing
- [ ] New tests added
- [ ] All tests pass
- [ ] Manual testing completed

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] No breaking changes (or documented)
```

### Review Process

1. **Automated Checks**: CI/CD runs tests and linting
2. **Code Review**: Maintainers review code and design
3. **Feedback**: Address reviewer comments
4. **Approval**: Maintainer approves changes
5. **Merge**: Changes are merged to main branch

### Response Expectations

- Initial response within 48 hours
- Detailed review within 1 week
- Small PRs reviewed faster than large ones
- Bug fixes prioritized over features

## Issue Guidelines

### Bug Reports

Use the bug report template:

```markdown
**Bug Description**
Clear description of the bug.

**Steps to Reproduce**
1. Step one
2. Step two
3. Step three

**Expected Behavior**
What should happen.

**Actual Behavior**
What actually happens.

**Environment**
- OS:
- Go version:
- Python version:
- Spacy version:
- Model(s) used:

**Additional Context**
Screenshots, logs, etc.
```

### Feature Requests

Use the feature request template:

```markdown
**Feature Description**
Clear description of the proposed feature.

**Motivation**
Why is this feature needed?

**Use Case**
How would this feature be used?

**Proposed Implementation**
Ideas for how to implement (optional).

**Additional Context**
Examples, references, etc.
```

### Issue Labels

- `bug`: Something isn't working
- `enhancement`: New feature or request
- `documentation`: Improvements to docs
- `good-first-issue`: Good for newcomers
- `help-wanted`: Extra attention needed
- `question`: Further information requested

## Community

### Communication Channels

- **GitHub Issues**: Bug reports and feature requests
- **GitHub Discussions**: General questions and ideas
- **Pull Requests**: Code contributions and reviews

### Getting Help

1. Check existing documentation
2. Search closed issues
3. Ask in GitHub Discussions
4. Create a new issue if needed

### Recognition

Contributors are recognized in:
- README contributors section
- Release notes
- Project documentation

## Release Process

### Version Numbering

We follow [Semantic Versioning](https://semver.org/):
- **MAJOR**: Incompatible API changes
- **MINOR**: New functionality, backward compatible
- **PATCH**: Bug fixes, backward compatible

### Release Checklist

- [ ] All tests passing
- [ ] Documentation updated
- [ ] CHANGELOG updated
- [ ] Version number bumped
- [ ] Git tag created
- [ ] GitHub release created

## Advanced Contributing

### Adding Language Support

To add support for a new language:

1. **Test Model Availability**: Ensure Spacy model exists
2. **Add Language Tests**: Create test cases in `spacy_multilang_test.go`
3. **Update Documentation**: Add language to supported languages list
4. **Test Thoroughly**: Verify all features work with the new language

### Performance Optimization

For performance improvements:

1. **Benchmark First**: Establish baseline performance
2. **Profile Code**: Identify bottlenecks using go tools
3. **Optimize Carefully**: Maintain correctness while improving speed
4. **Benchmark After**: Verify improvements
5. **Document Changes**: Explain optimization approach

### Adding New Features

For new features:

1. **Design Document**: Create RFC for significant features
2. **API Design**: Consider backward compatibility
3. **Implementation**: Follow coding standards
4. **Testing**: Comprehensive test coverage
5. **Documentation**: Update all relevant docs
6. **Examples**: Provide usage examples

## FAQ

**Q: How do I set up development on Windows?**
A: Use WSL2 for the best experience, or follow the native Windows setup in the installation guide.

**Q: Can I add support for other NLP libraries?**
A: This project focuses on Spacy bindings. For other libraries, consider creating a separate project.

**Q: How do I handle failing CI tests?**
A: Check the CI logs, reproduce locally, fix the issues, and push the fixes.

**Q: What if my PR is taking too long to review?**
A: Feel free to politely ping the maintainers after a week.

**Q: Can I work on multiple issues simultaneously?**
A: Yes, but create separate branches and PRs for each issue.

## Thank You!

Thank you for contributing to Go-Spacy! Your contributions help make natural language processing more accessible to the Go community.

For questions about contributing, feel free to reach out via GitHub issues or discussions.