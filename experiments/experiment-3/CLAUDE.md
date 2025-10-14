# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview
<!-- auto-generated-start:overview -->
Atomix distributed consensus experiment using Go SDK to test distributed key-value map primitives in a Kubernetes environment with multi-raft consensus clusters.
<!-- auto-generated-end:overview -->

## Key Objectives
<!-- auto-generated-start:objectives -->
- Test Atomix distributed primitives using Go SDK
- Demonstrate consensus-based storage with multi-raft clusters
- Validate distributed map operations in Kubernetes environment
<!-- auto-generated-end:objectives -->

## Project Structure

```
.
├── CLAUDE.md          # This file - project instructions for Claude
├── .claude/           # Claude Code configuration (auto-generated)
│   ├── agents/        # Project-specific agent overrides
│   └── commands/      # Custom slash commands for Claude Code
├── claude/            # Claude Code project organization
│   ├── agents/        # Custom agents for specialized tasks
│   ├── docs/          # Project documentation
│   ├── plans/         # Project plans and architectural documents
│   └── tickets/       # Task tickets and issues
└── [your project files and directories]
```

## Development Guidelines

### Code Style

- Follow existing code conventions in the project
- Use consistent naming patterns
- Maintain clean, readable code

### Testing

- Run tests before committing changes
- Add tests for new functionality
- Ensure all tests pass

### Git Workflow

- Create descriptive commit messages
- Keep commits focused and atomic
- Review changes before committing

## Commit Convention and Pull Request Guidelines

### Commit Message Format
Follow the conventional commits specification:
```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, missing semicolons, etc.)
- `refactor`: Code refactoring without changing functionality
- `test`: Adding or modifying tests
- `chore`: Maintenance tasks (updating dependencies, build process, etc.)
- `perf`: Performance improvements

**Examples:**
```
feat(auth): add password reset functionality
fix(api): handle null values in user response
docs: update API documentation for book endpoints
refactor(frontend): extract BookTable into separate components
chore(deps): update FastAPI to 0.104.1
```

### Pull Request Guidelines

**PR Title**: Use the same format as commit messages

**PR Description Template:**
```markdown
## Summary
Brief description of what this PR does and why it's needed.

## Changes
- List of specific changes made
- Technical implementation details if relevant

## Testing
- [ ] Tests pass (if applicable)
- [ ] Manual testing completed
- [ ] No console errors or warnings

## Manual Testing Steps
1. Describe steps to manually test the feature
2. Expected behavior and edge cases tested

## Screenshots (if UI changes)
Attach relevant screenshots here

## Related Issues
Closes #XXX (if applicable)

## Checklist
- [ ] Code follows project conventions
- [ ] Self-documented code without unnecessary comments
- [ ] All tests pass
- [ ] Documentation updated if needed
- [ ] No sensitive information exposed
```

## Common Commands
<!-- auto-generated-start:commands -->
```bash
# Complete deployment workflow
./run.sh                    # Full setup: minikube restart, helm install, docker build, k8s deploy

# Individual commands
minikube start              # Start local Kubernetes cluster
minikube delete             # Clean up minikube environment

# Atomix runtime setup
helm install -n kube-system atomix-runtime atomix/atomix-runtime --wait

# Build and load application
docker build -t experiment-client:local .
minikube image load experiment-client:local

# Kubernetes deployment
kubectl apply -f test.yaml      # Deploy consensus store (3 replicas, 3 groups)
kubectl apply -f storage-profile.yaml      # Configure storage profile bindings
kubectl apply -f deployment.yaml           # Deploy experiment client

# Go development
go mod download             # Download dependencies
go build -o atomix-app ./main.go          # Build the application
go run main.go             # Run locally (requires Atomix runtime)

# Monitoring
kubectl get multiraftclusters # Check consensus cluster status
kubectl logs -f deployment/atomix-experiment  # Monitor application logs
kubectl describe storageprofile atomix-experiment  # Check storage profile
```
<!-- auto-generated-end:commands -->

## Architecture & Technical Context

### Atomix Distributed System
This is an experimental implementation using the Atomix distributed systems framework:
- **Consensus Protocol**: Multi-raft clusters for distributed consensus
- **Storage**: ConsensusStore with 3 replicas distributed across 3 groups
- **Primitives**: Testing distributed Map primitive with string key-value pairs
- **Runtime**: Requires Atomix runtime deployed via Helm in kube-system namespace

### Kubernetes Configuration
- **ConsensusStore** (`consensus-store.yaml`): Defines the distributed storage backend
  - 3 replicas with 3 consensus groups
  - 8Gi persistent volumes with ReadWriteOnce access
  - Uses standard storage class
- **StorageProfile** (`storage-profile.yaml`): Binds the consensus store to the application
- **Deployment** (`deployment.yaml`): Client application with Atomix proxy injection
  - Proxy injection enabled: `proxy.atomix.io/inject: "true"`
  - Profile binding: `proxy.atomix.io/profile: "atomix-experiment"`

### Application Structure
- **main.go**: Go application demonstrating distributed Map operations
  - Creates a distributed map named "test-map"
  - Performs Put/Get operations to validate consensus behavior  
  - Tests multiple key-value pairs to demonstrate distributed storage
- **Dependencies**: Uses `github.com/atomix/go-sdk v0.10.0` for Atomix primitives

### Development Environment
- **Container**: Alpine-based Go 1.24.6 container
- **Local Development**: Requires minikube for Kubernetes simulation
- **Deployment**: Full automation via `run.sh` script handles complete environment setup

## Agents

See @claude/agents/README.md for available agents and their purposes

## Agent Orchestration

After adding the agents you want to in `./claude/agents` folder, setup the workflow for Claude code to follow

## Custom Commands

Custom slash commands are available in `.claude/commands/`:
- **/update-claude-md** - Automatically updates this file with project-specific information
- See `.claude/commands/README.md` for creating your own commands

## Tickets

See @claude/tickets/README.md for ticket format and management approach

### Ticket Management
- **Ticket List**: Maintain @claude/tickets/ticket-list.md as a centralized index of all tickets
- **Update ticket-list.md** whenever you:
  - Create a new ticket (add to appropriate priority section)
  - Change ticket status (update emoji and move if completed)
  - Complete a ticket (move to completed section with date)
- **Status Emojis**: 🔴 Todo | 🟡 In Progress | 🟢 Done | 🔵 Blocked | ⚫ Cancelled

## Plans

See @claude/plans/README.md for planning documents and architectural decisions

## Development Context

- See @claude/docs/ROADMAP.md for current status and next steps
- Task-based development workflow with tickets in `claude/tickets` directory
- Use `claude/plans` directory for architectural decisions and implementation roadmaps

## Important Instructions

Before starting any task:

1. **Confirm understanding**: Always confirm you understand the request and outline your plan before proceeding
2. **Ask clarifying questions**: Never make assumptions - ask questions when requirements are unclear
3. **Create planning documents**: Before implementing any code or features, create a markdown file documenting the approach
4. **Use plans directory**: When discussing ideas or next steps, create timestamped files in the plans directory (e.g., `claude/plans/next-steps-YYYY-MM-DD-HH-MM-SS.md`) to maintain a record of decisions
5. **No code comments**: Never add comments to any code you write - code should be self-documenting
6. **Maintain ticket list**: Always update @claude/tickets/ticket-list.md when creating, updating, or completing tickets to maintain a clear project overview

## Additional Notes
<!-- auto-generated-start:notes -->
atomix.Map cannot be used as a type in a struct. It must be instantiated every use.
<!-- auto-generated-end:notes -->