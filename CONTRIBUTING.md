# Contributing Guide

Welcome to the **High Performance Request Processing System** project.

This document explains how every team member should contribute to the project.

---

# Project Workflow

We follow a simple Git workflow.

```
main
│
└── develop
      │
      ├── feature/backend
      ├── feature/round-robin
      ├── feature/weighted-round-robin
      ├── feature/least-connections
      ├── feature/adaptive
      ├── feature/cache
      ├── feature/rate-limiter
      └── feature/benchmarking
```

### Branches

**main**

- Stable code
- Always deployable
- Never commit directly

---

**develop**

- Integration branch
- All completed features are merged here first
- Never commit directly

---

**feature/**

Each developer works on their own feature branch.

Examples:

```
feature/backend

feature/round-robin

feature/weighted-round-robin

feature/least-connections

feature/adaptive
```

---

# First Time Setup

Clone the repository

```bash
git clone https://github.com/<username>/high-performance-request-system.git

cd high-performance-request-system
```

Fetch all branches

```bash
git fetch
```

Switch to develop

```bash
git checkout develop
```

If develop doesn't exist locally

```bash
git checkout -b develop origin/develop
```

---

# Creating Your Feature Branch

Create your own branch from develop.

Example

```bash
git checkout -b feature/round-robin
```

Push it once

```bash
git push -u origin feature/round-robin
```

After this, simply use

```bash
git push
```

---

# Daily Workflow

Before starting work

```bash
git checkout develop

git pull
```

Switch to your branch

```bash
git checkout feature/<your-feature>
```

Merge latest develop

```bash
git merge develop
```

Continue coding.

---

# Saving Your Work

Check status

```bash
git status
```

Stage files

```bash
git add .
```

Commit

```bash
git commit -m "Meaningful commit message"
```

Examples

```
Implement Round Robin algorithm

Fix connection counter

Add unit tests

Refactor backend handlers
```

Push

```bash
git push
```

Push frequently.

---

# Pull Requests

Create a Pull Request only when

- Feature is complete
- Code compiles
- Basic testing is complete

Base branch

```
develop
```

Compare branch

```
feature/<your-feature>
```

Never create a Pull Request directly to main.

---

# Updating Your Branch

Whenever another feature is merged into develop

Run

```bash
git checkout develop

git pull

git checkout feature/<your-feature>

git merge develop
```

Resolve conflicts if any.

Continue coding.

---

# Merge Conflicts

Merge conflicts happen when two branches modify the same part of the same file.

Git cannot decide which version to keep.

If a conflict occurs

1. Read the conflicting code
2. Decide the correct version
3. Remove conflict markers
4. Save
5. Commit

Do not panic.

Merge conflicts are normal.

---

# Branch Ownership

Each feature has one primary owner.

| Branch | Owner |
|---------|-------|
| feature/backend | Tejas |
| feature/round-robin | Member 1 |
| feature/weighted-round-robin | Member 2 |
| feature/least-connections | Member 3 |
| feature/adaptive | Shared |
| feature/cache | Shared |
| feature/rate-limiter | Shared |

If you need to modify another person's feature

- Inform them
- Switch to their branch
- Commit there
- Push to the same branch

Do NOT modify their feature from your own branch.

---

# Team Rules

✅ Never commit directly to main

✅ Never commit directly to develop

✅ Work only on your feature branch

✅ Commit frequently

✅ Push frequently

✅ Pull before starting work

✅ Merge develop into your feature branch regularly

✅ Open Pull Requests only when the feature is ready

---

# Project Structure

```
high-performance-request-system/

cmd/
    backend/
    loadbalancer/

internal/

    algorithms/

        strategy.go
        round_robin.go
        weighted_round_robin.go
        least_connections.go
        adaptive.go

    models/

        server.go
        metrics.go

    handlers/

    proxy/

    database/

    cache/

    ratelimiter/

    metrics/

    config/

tests/

benchmarks/

docs/
```

---

# Responsibilities

### Tejas

- Backend
- HTTP Server
- PostgreSQL
- Reverse Proxy
- Integration
- Docker

---

### Member 1

- Round Robin
- Unit Tests
- Documentation

---

### Member 2

- Weighted Round Robin
- Unit Tests
- Documentation

---

### Member 3

- Least Connections
- Unit Tests
- Documentation

---

### Entire Team

- Adaptive Algorithm
- Redis Cache
- Rate Limiter
- Benchmarking
- Final Documentation
- Performance Testing

---

# Git Commands Cheat Sheet

Clone

```bash
git clone <repo-url>
```

Fetch

```bash
git fetch
```

See branches

```bash
git branch
```

See all branches

```bash
git branch -a
```

Switch branch

```bash
git checkout <branch>
```

Create branch

```bash
git checkout -b feature/<name>
```

Check changes

```bash
git status
```

Stage files

```bash
git add .
```

Commit

```bash
git commit -m "message"
```

Push

```bash
git push
```

Pull

```bash
git pull
```

Merge develop

```bash
git merge develop
```

---

# Common Mistakes

❌ Working directly on main

❌ Working directly on develop

❌ Forgetting to pull latest develop

❌ Huge commits after several days

❌ Pushing broken code

❌ Opening Pull Requests for unfinished work

---

# Contribution Philosophy

- Learn the concept before implementing.
- Keep commits small and meaningful.
- Ask questions early if blocked.
- Write clean and readable code.
- Test before opening a Pull Request.
- Help teammates when needed.

---

Happy Coding 🚀