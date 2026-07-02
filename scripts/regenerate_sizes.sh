#!/bin/bash
# Regenerate provider/sizes.go from the live SHC catalog.
# Run from the terraform-provider-shc repo root.
#
# Usage:
#   SHC_API_KEY=... ./scripts/regenerate_sizes.sh
#
# Or if shc-toolkit is a sibling repo:
#   python3 ../shc-toolkit/scripts/generate_sizes.py --format go --output provider/sizes.go

set -e

TOOLKIT_GEN="../shc-toolkit/scripts/generate_sizes.py"

if [ -f "$TOOLKIT_GEN" ]; then
    python3 "$TOOLKIT_GEN" --format go --output provider/sizes.go
else
    pip install git+https://github.com/Amperstrand/shc-toolkit.git -q
    curl -sL https://raw.githubusercontent.com/Amperstrand/shc-toolkit/main/scripts/generate_sizes.py \
        | python3 - --format go --output provider/sizes.go
fi

echo "Done. Review the diff and commit."
