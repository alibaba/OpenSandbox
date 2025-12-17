name: Bug report
description: Create a report to help us improve
labels: [bug]
body:
  - type: textarea
    id: summary
    attributes:
      label: Summary
      description: What happened? What did you expect?
    validations:
      required: true
  - type: textarea
    id: repro
    attributes:
      label: Steps to Reproduce
      description: Step-by-step instructions to reproduce the issue.
      placeholder: |
        1. ...
        2. ...
        3. ...
    validations:
      required: true
  - type: input
    id: version
    attributes:
      label: Version/Commit
      description: Release tag or commit hash.
  - type: input
    id: env
    attributes:
      label: Environment
      description: OS, architecture, runtime versions, etc.
  - type: textarea
    id: logs
    attributes:
      label: Logs / Screenshots
      description: Add relevant logs or screenshots.
  - type: checkboxes
    id: regress
    attributes:
      label: Regression?
      options:
        - label: This worked in a previous version
