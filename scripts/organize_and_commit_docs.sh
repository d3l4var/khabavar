#!/usr/bin/env bash
# organize_and_commit_docs.sh — copies all spec docs, runbooks, and source material
# into the repo's docs/ tree, then commits them in logical groups.
set -euo pipefail
cd /home/z/my-project

echo "=== 1. Create docs/specs/ (canonical R-XX reconciled specs) ==="
mkdir -p docs/specs
SRC="scripts/drive_prep/reconciled"
cp "$SRC/KHABAVAR — R-01 · Master Project State.txt"                docs/specs/R-01-master-project-state.md
cp "$SRC/KHABAVAR — R-04 · Product Catalog v1.0.txt"                docs/specs/R-04-product-catalog.md
cp "$SRC/KHABAVAR — R-05 · Supplier Evaluation & Unit Economics.txt" docs/specs/R-05-supplier-evaluation-unit-economics.md
cp "$SRC/KHABAVAR — R-06 · Commerce, Order & Fulfillment Spec.txt"  docs/specs/R-06-commerce-order-fulfillment.md
cp "$SRC/KHABAVAR — R-07 · API Contract (OpenAPI v1.1.0).yaml"      docs/specs/R-07-openapi-contract.yaml
cp "$SRC/KHABAVAR — R-11 · IAM Matrix v1.1.txt"                     docs/specs/R-11-iam-matrix.md
cp "$SRC/KHABAVAR — R-14 · DB Migrations v2.0.sql"                  docs/specs/R-14-db-migrations-v2.sql
cp "$SRC/KHABAVAR — R-14 · DB Seed Baseline v1.1.txt"               docs/specs/R-14-db-seed-baseline.md
cp "$SRC/KHABAVAR — R-16 · SAD v2.1.txt"                            docs/specs/R-16-sad-v2.1.md
cp "$SRC/KHABAVAR — R-17 · Test Strategy v1.1.txt"                  docs/specs/R-17-test-strategy.md
cp "$SRC/KHABAVAR — R-18 · Deployment Spec v1.1.txt"                docs/specs/R-18-deployment-spec.md
cp "$SRC/KHABAVAR — R-19 · Implementation Roadmap v1.0.txt"         docs/specs/R-19-implementation-roadmap.md
cp "$SRC/KHABAVAR — R-CI · CI Pipeline & Quality Gates Spec v1.1.txt" docs/specs/R-CI-pipeline-quality-gates.md
cp "$SRC/KHABAVAR — R-INDEX · Reading Path & Content Review v1.1.txt" docs/specs/R-INDEX-reading-path.md
echo "  -> $(ls docs/specs/ | wc -l) spec files"

echo ""
echo "=== 2. Create docs/source-material/ (original uploaded project docs) ==="
mkdir -p docs/source-material
cp upload/khabavar/01_MASTER_PROJECT_STATE.txt     docs/source-material/01-master-project-state.md
cp upload/khabavar/02_PRODUCT_CATALOG.txt          docs/source-material/02-product-catalog.md
cp upload/khabavar/03_SUPPLIER_EVAL.txt            docs/source-material/03-supplier-eval.md
cp upload/khabavar/04_BRAND_DESIGN.txt             docs/source-material/04-brand-design.md
cp upload/khabavar/05_CIAM.txt                     docs/source-material/05-ciam.md
cp upload/khabavar/06_COMMERCE_ORDER.txt           docs/source-material/06-commerce-order.md
cp upload/khabavar/07_DATABASE_ARCH.txt            docs/source-material/07-database-arch.md
cp upload/khabavar/08_DATABASE_DOMAIN_DESIGN.txt   docs/source-material/08-database-domain-design.md
cp upload/khabavar/09_IMPLEMENTATION_ROADMAP.txt   docs/source-material/09-implementation-roadmap.md
cp upload/khabavar/10_BIZ_ANALYSIS_TECH_SPEC.txt   docs/source-material/10-biz-analysis-tech-spec.md
cp upload/khabavar/11_SAD.txt                      docs/source-material/11-sad.md
echo "  -> $(ls docs/source-material/ | wc -l) source-material files"

echo ""
echo "=== 3. Update .gitignore (exclude .env, keep .env.example) ==="
cat > .gitignore <<'GITEOF'
# Secrets — never commit
.env

# Agent-managed skill library (not part of the project repo)
skills/
node_modules/

# Build artifacts
bin/
*.test
*.out
__pycache__/
*.pyc

# Editor / OS
.DS_Store
.idea/
.vscode/
GITEOF
echo "  -> .gitignore updated"

echo ""
echo "=== 4. Copy useful scripts (exclude __pycache__, api_parts scratch) ==="
# Don't copy the whole scripts/ dir — only the reusable build/verify scripts.
mkdir -p scripts
cp scripts/build_khabavar_repo.py      scripts_repo_build_khabavar_repo.py 2>/dev/null || true
# Keep scripts where they are; just ensure they're tracked.
echo "  -> scripts/ already in tree"

echo ""
echo "=== 5. Verify nothing sensitive leaks ==="
echo "--- .env tracked? (should be NO) ---"
git check-ignore .env && echo "  .env is gitignored ✓" || echo "  WARNING: .env NOT ignored!"
echo "--- docs/specs file count ---"
ls docs/specs/ | wc -l
echo "--- docs/source-material file count ---"
ls docs/source-material/ | wc -l

echo ""
echo "=== DONE — ready to git add + commit ==="
