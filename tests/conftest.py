import os
import tempfile
from pathlib import Path

import pytest


@pytest.fixture
def tmp_vault(tmp_path):
    """Create a temporary vault with sample notes."""
    contact = tmp_path / "12-CRM" / "Contacts"
    contact.mkdir(parents=True)
    (contact / "Shah Ali.md").write_text(
        "---\n"
        "title: Shah Ali\n"
        "type: contact\n"
        "entity: [properties]\n"
        "status: active\n"
        "---\n\n"
        "## Context\n\nProperty sourcing agent.\n"
    )

    meetings = tmp_path / "20-Meetings"
    meetings.mkdir()
    long_content = (
        "---\n"
        "title: IT Systems Progress Update\n"
        "type: meeting\n"
        "entity: [diixtra]\n"
        "status: done\n"
        "---\n\n"
        "## Agenda\n\n"
        + "Discussion about IDOX migration timeline and milestones. " * 20
        + "\n\n## Action Items\n\n"
        + "- Complete phase 2 migration by November. " * 20
        + "\n\n## Decisions\n\n"
        + "- Approved the new hosting configuration. " * 20
    )
    (meetings / "2025-10-15 - IT Systems Progress Update.md").write_text(long_content)

    knowledge = tmp_path / "50-Knowledge" / "Library"
    knowledge.mkdir(parents=True)
    no_heading_content = (
        "---\n"
        "title: Business Framework\n"
        "type: framework\n"
        "entity: [group]\n"
        "status: active\n"
        "---\n\n"
        + "This is a business framework for evaluating acquisitions. " * 40
    )
    (knowledge / "Business Framework.md").write_text(no_heading_content)

    (tmp_path / "plain.md").write_text("Just some text without frontmatter.\n")
    (tmp_path / "malformed.md").write_text("---\ntitle: [broken\n---\nBody text.\n")

    dashboards = tmp_path / "80-Dashboards"
    dashboards.mkdir()
    (dashboards / "Home.md").write_text("---\ntitle: Home\n---\nDataview stuff.\n")

    return tmp_path
