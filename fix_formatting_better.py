#!/usr/bin/env python3
import re

# Read the file
with open('site/src/contexts/ProxyContext.tsx', 'r') as f:
    lines = f.readlines()

# Fix the file line by line
fixed_lines = []
i = 0
while i < len(lines):
    line = lines[i]
    
    # Fix line breaks in comments
    if 'comes fro\\' in line and i + 1 < len(lines) and 'm either:' in lines[i + 1]:
        fixed_lines.append(line.replace('comes fro\\', 'comes from either:'))
        i += 2  # Skip the next line
        continue
    elif line.strip() == 'm either:':
        i += 1  # Skip this line
        continue
    
    # Fix other line breaks
    if 'valu\\' in line and i + 1 < len(lines) and 'nes are sourced' in lines[i + 1]:
        fixed_lines.append(line.replace('valu\\', 'values are sourced'))
        i += 2
        continue
    elif 'nes are sourced' in line.strip():
        i += 1
        continue
        
    if 'comes\n' in line and i + 1 < len(lines) and ' from local storage' in lines[i + 1]:
        fixed_lines.append(line.replace('comes\n', 'comes from local storage'))
        i += 2
        continue
    elif line.strip() == ' from local storage':
        i += 1
        continue
        
    if 'migrati\\' in line and i + 1 < len(lines) and 'on' in lines[i + 1]:
        fixed_lines.append(line.replace('migrati\\', 'migration'))
        i += 2
        continue
    elif line.strip() == 'on' and i > 0 and 'migrati' in lines[i-1]:
        i += 1
        continue
    
    # Fix indentation - replace 8 spaces with tabs at the beginning of lines
    if line.startswith('        '):
        line = line.replace('        ', '\t', 1)
    
    fixed_lines.append(line)
    i += 1

# Write the file back
with open('site/src/contexts/ProxyContext.tsx', 'w') as f:
    f.writelines(fixed_lines)

print("Fixed formatting issues")
