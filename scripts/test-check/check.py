import argparse
import json
from pathlib import Path
from dataclasses import dataclass
from typing import Dict

@dataclass
class TestResult:
    Time: str
    Action: str
    Package: str
    Test: str
    Elapsed: float

def load_and_process_files(directory: Path) -> Dict[str, Dict[str, int]]:
    test_results: Dict[str, Dict[str, int]] = {}
    
    for file in directory.glob('*-timings.json'):
        with file.open() as f:
            for line in f:
                try:
                    data = json.loads(line)
                    if data.get('Test') is None:
                        continue

                    result = TestResult(
                        Time=data['Time'],
                        Action=data['Action'],
                        Package=data['Package'],
                        Test=data['Test'],
                        Elapsed=data['Elapsed']
                    )
                    
                    assert isinstance(result.Time, str)
                    assert isinstance(result.Action, str)
                    assert isinstance(result.Package, str)
                    assert isinstance(result.Test, str)
                    assert isinstance(result.Elapsed, (int, float))
                    
                    test_key = f"{result.Package}/{result.Test}"
                    if test_key not in test_results:
                        test_results[test_key] = {'pass': 0, 'fail': 0, 'skip': 0}
                    
                    test_results[test_key][result.Action] += 1
                    
                except (json.JSONDecodeError, KeyError, AssertionError) as e:
                    print(f"Error processing line `{line}` in {file}: {e}")
                    continue
    
    return test_results

def print_failed_tests(test_results: Dict[str, Dict[str, int]]):
    for test, results in test_results.items():
        if results['pass'] == 0 and results['skip'] == 0:
            print(f"Test never succeeded: {test}")

def print_flaky_tests(test_results: Dict[str, Dict[str, int]]):
    for test, results in test_results.items():
        if results['pass'] > 0 and results['fail'] > 0:
            print(f"Test is flaky: {test}")

def print_score_totals(test_results: Dict[str, Dict[str, int]]):
    totals = [sum(results.values()) for results in test_results.values()]
    total_counts = {}
    for total in totals:
        total_counts[total] = total_counts.get(total, 0) + 1
    print(f"Total counts: {total_counts}")

def main():
    parser = argparse.ArgumentParser(description="Process test timing files.")
    parser.add_argument('directory', type=Path, help="Directory containing timing files")
    args = parser.parse_args()

    if not args.directory.is_dir():
        print(f"Error: {args.directory} is not a valid directory")
        return

    test_results = load_and_process_files(args.directory)
    print_failed_tests(test_results)
    print_flaky_tests(test_results)
    print_score_totals(test_results)

if __name__ == "__main__":
    main()
