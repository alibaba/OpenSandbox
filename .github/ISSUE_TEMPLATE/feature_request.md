name: Feature request
description: Suggest an idea for this project
labels: [enhancement]
body:
  - type: textarea
    id: motivation
    attributes:
      label: Motivation
      description: What problem does this solve? Who benefits?
    validations:
      required: true
  - type: textarea
    id: proposal
    attributes:
      label: Proposed Solution
      description: How would you like to see this addressed?
      placeholder: |
        - API/CLI changes
        - UX expectations
        - Constraints or assumptions
  - type: textarea
    id: alternatives
    attributes:
      label: Alternatives
      description: Other approaches you considered.
  - type: textarea
    id: additional
    attributes:
      label: Additional Context
      description: Links, diagrams, prior art, etc.
