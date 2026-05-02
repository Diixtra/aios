---
title: "Add a 'Local development' section to README"
slug: 001-add-readme-section
difficulty: easy
agentType: code-pr
---

The repo's README is missing a "Local development" section. Add one between the existing "Installation" and "Contributing" sections (or at the end if those don't exist).

Required content:

- One-line summary of how to clone and bootstrap the repo (`git clone … && just bootstrap` or whatever the repo's actual bootstrap step is)
- The command to run tests locally
- The command to run the build locally
- A short note (~2 lines) on how to add a new test

Match the existing README's tone and heading style. Don't introduce new headings outside the "Local development" section.

**Acceptance:** `git diff main -- README.md` shows the new section, no other files changed, tests still pass.
