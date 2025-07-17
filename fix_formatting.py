#!/usr/bin/env python3
import re

# Read the file
with open('site/src/contexts/ProxyContext.tsx', 'r') as f:
    content = f.read()

# Fix line breaks in comments
content = re.sub(r'comes fro\\\nm either:', 'comes from either:', content)
content = re.sub(r'These valu\\\nes are sourced from', 'These values are sourced from', content)
content = re.sub(r'comes\n from local storage', 'comes from local storage', content)
content = re.sub(r'from this\n', 'from this ', content)
content = re.sub(r'migrati\\\non', 'migration', content)

# Fix indentation - replace 8 spaces with tabs
content = re.sub(r'^        ', '\t', content, flags=re.MULTILINE)

# Write the file back
with open('site/src/contexts/ProxyContext.tsx', 'w') as f:
    f.write(content)

print("Fixed formatting issues")
