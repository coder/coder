#!/usr/bin/env python3

import argparse
from pathlib import Path
import re


def find_test_locations(test_names_file: Path, test_dir: Path):
    # Load test names from file
    test_names: dict[str, str] = {}
    for line in test_names_file.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        test_name = line.split(",")[-1]
        test_names[line] = test_name

    # Find all test files
    test_files = list(test_dir.rglob("*_test.go"))

    # Search for each test name
    test_locations = {}
    for test_key, test_name in test_names.items():
        test_name_parts = test_name.split("/")
        test_name_suffixes = [test_name_parts[i:] for i in range(len(test_name_parts))]
        for suffix in test_name_suffixes:
            match_found = False
            for test_file in test_files:
                contents = test_file.read_text()
                for line_num, line in enumerate(contents.splitlines(), 1):
                    # Look for exact test name match
                    if f"s.Run(\"{'/'.join(suffix)}\"" in line:
                        if test_name not in test_locations:
                            test_locations[test_key] = []
                        test_locations[test_key].append((test_file, line_num))
                        match_found = True
            if match_found:
                break

    # Print test names that don't appear exactly once
    for test_key, locations in test_locations.items():
        if len(locations) != 1:
            print(f"Test '{test_names[test_key]}' found in multiple locations:")
            for file_path, line_num in locations:
                print(f"  {file_path}:{line_num}")

    for test_key, test_name in test_names.items():
        if test_key not in test_locations:
            print(f"Test '{test_name}' not found in any test file")

    # Create insertions_needed from test_locations
    insertions_needed: dict[Path, list[int]] = {}
    for locations in test_locations.values():
        for file_path, line_num in locations:
            if file_path not in insertions_needed:
                insertions_needed[file_path] = []
            insertions_needed[file_path].append(line_num)

    # Process each file's insertions
    for file_path, line_numbers in insertions_needed.items():
        # Sort line numbers to process in order
        line_numbers.sort()
        
        # Read file contents
        lines = file_path.read_text().splitlines()
        
        # Insert new lines, keeping track of offset
        offset = 0
        for line_num in line_numbers:
            # Adjust line number by current offset
            adjusted_line_num = line_num + offset
            
            # Insert the new line after the target line
            lines.insert(adjusted_line_num, "\tdbtestutil.DisableForeignKeys(s.T(), db)")
            offset += 1
        
        # Write back to file
        file_path.write_text('\n'.join(lines))

    # Print summary
    for file_path, line_numbers in insertions_needed.items():
        print(f"Modified {file_path}: inserted {len(line_numbers)} lines")



def main():
    parser = argparse.ArgumentParser(description="Find test locations in Go files")
    parser.add_argument("test_names_file", type=Path, help="File containing test names")
    parser.add_argument("test_dir", type=Path, help="Directory containing test files")
    args = parser.parse_args()

    find_test_locations(args.test_names_file, args.test_dir)


if __name__ == "__main__":
    main()
