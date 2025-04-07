#!/usr/bin/env python3
import sys
import json
import os
import re

files_to_analyze = sys.stdin.read().strip().split('\n')
doc_structure = {}

for file_path in files_to_analyze:
    if not file_path or not file_path.endswith('.md') or not os.path.isfile(file_path):
        continue
        
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            content = f.read()
            
        # Extract title (first h1)
        title_match = re.search(r'^# (.+)$', content, re.MULTILINE)
        title = title_match.group(1) if title_match else 'Untitled'
        
        # Count headings
        h1_count = len(re.findall(r'^# ', content, re.MULTILINE))
        h2_count = len(re.findall(r'^## ', content, re.MULTILINE))
        h3_count = len(re.findall(r'^### ', content, re.MULTILINE))
        
        doc_structure[file_path] = {
            'title': title,
            'headings': {
                'h1': h1_count,
                'h2': h2_count,
                'h3': h3_count
            }
        }
        
        print(f'Analyzed {file_path}: H1={h1_count}, H2={h2_count}, H3={h3_count}, Title="{title}"', file=sys.stderr)
    except Exception as e:
        print(f'Error analyzing {file_path}: {str(e)}', file=sys.stderr)

# Write JSON output
with open('.github/temp/doc_structure.json', 'w', encoding='utf-8') as f:
    json.dump(doc_structure, f, indent=2)

print(json.dumps(doc_structure))