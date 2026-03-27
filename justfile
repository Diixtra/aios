# Diixtra repo management recipes

# Create a new repo from the template
repo-new name visibility="private":
    #!/usr/bin/env bash
    set -euo pipefail
    gh repo create "Diixtra/{{ name }}" --template Diixtra/diixtra-template "--{{ visibility }}" --clone
    if [ "{{ visibility }}" = "private" ]; then
        cd "{{ name }}"
        git checkout -b chore/strip-public-files
        rm -f LICENSE .github/FUNDING.yml .github/workflows/scorecard.yml
        git add LICENSE .github/FUNDING.yml .github/workflows/scorecard.yml
        git commit -m "chore: strip public-only files for private repo"
        git push -u origin chore/strip-public-files
        gh pr create --title "chore: strip public-only files" \
            --body "Removes public-only files (LICENSE, FUNDING.yml, scorecard.yml) for private repo."
        cd ..
    fi
    echo "Repo Diixtra/{{ name }} created successfully."

# Toggle an existing repo's visibility and update file profile
repo-toggle name visibility:
    #!/usr/bin/env bash
    set -euo pipefail
    gh repo edit "Diixtra/{{ name }}" --visibility "{{ visibility }}"
    TMPDIR=$(mktemp -d)
    gh repo clone "Diixtra/{{ name }}" "$TMPDIR/{{ name }}"
    cd "$TMPDIR/{{ name }}"
    if [ "{{ visibility }}" = "private" ]; then
        git checkout -b chore/strip-public-files
        rm -f LICENSE .github/FUNDING.yml .github/workflows/scorecard.yml
        git add LICENSE .github/FUNDING.yml .github/workflows/scorecard.yml
        git commit -m "chore: strip public-only files for private repo"
        git push -u origin chore/strip-public-files
        gh pr create --title "chore: strip public-only files" \
            --body "Removes public-only files after visibility change to private."
    else
        git checkout -b chore/add-public-files
        gh api repos/Diixtra/diixtra-template/contents/LICENSE --jq '.content' | base64 --decode > LICENSE
        mkdir -p .github .github/workflows
        gh api repos/Diixtra/diixtra-template/contents/.github/FUNDING.yml --jq '.content' | base64 --decode > .github/FUNDING.yml
        gh api repos/Diixtra/diixtra-template/contents/.github/workflows/scorecard.yml --jq '.content' | base64 --decode > .github/workflows/scorecard.yml
        git add LICENSE .github/FUNDING.yml .github/workflows/scorecard.yml
        git commit -m "chore: add public-only files for public repo"
        git push -u origin chore/add-public-files
        gh pr create --title "chore: add public-only files" \
            --body "Adds public-only files (LICENSE, FUNDING.yml, scorecard.yml) after visibility change to public."
    fi
    rm -rf "$TMPDIR"
    echo "Visibility for Diixtra/{{ name }} toggled to {{ visibility }}."

# Bootstrap an existing repo with template files (non-destructive, opens PR)
repo-bootstrap name:
    #!/usr/bin/env bash
    set -euo pipefail
    TMPDIR=$(mktemp -d)
    gh repo clone "Diixtra/{{ name }}" "$TMPDIR/{{ name }}"
    gh repo clone Diixtra/diixtra-template "$TMPDIR/template"
    cd "$TMPDIR/{{ name }}"
    git checkout -b chore/add-project-scaffolding
    # Copy template files that don't already exist (skip overwriting)
    TEMPLATE_FILES=$(cd "$TMPDIR/template" && find . -type f \
        -not -path './.git/*' -not -name 'justfile' | sort)
    ADDED=""
    for f in $TEMPLATE_FILES; do
        if [ ! -f "$f" ]; then
            mkdir -p "$(dirname "$f")"
            cp "$TMPDIR/template/$f" "$f"
            ADDED="$ADDED $f"
        fi
    done
    if [ -z "$ADDED" ]; then
        echo "No new files to add for {{ name }}. Skipping."
        rm -rf "$TMPDIR"
        exit 0
    fi
    # Strip public-only files if private
    VISIBILITY=$(gh repo view "Diixtra/{{ name }}" --json visibility --jq '.visibility')
    if [ "$VISIBILITY" = "PRIVATE" ]; then
        rm -f LICENSE .github/FUNDING.yml .github/workflows/scorecard.yml
    fi
    # Stage only the files that were added
    for f in $ADDED; do
        git add "$f" 2>/dev/null || true
    done
    git commit -m "chore: add project scaffolding from diixtra-template"
    git push -u origin chore/add-project-scaffolding
    gh pr create --title "chore: add project scaffolding" \
        --body "Adds baseline project files from diixtra-template. Only new files added — existing files were not overwritten."
    rm -rf "$TMPDIR"
    echo "Bootstrap PR created for Diixtra/{{ name }}."
