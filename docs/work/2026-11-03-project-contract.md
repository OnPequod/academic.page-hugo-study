# Project links contract

Project pages render the API's ordered titled links. Tools are no longer
rendered, and the adapter uses only a project's start date for its date. The
fixture schema and project records match the new API contract.

Validation: make test passed against the mock API for all four site types,
including fixture-schema validation, Hugo builds, project links, and absence of
the removed tools field. No production API writes or deployment occurred.
