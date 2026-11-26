#!/usr/bin/env python3
"""Bump the version in version.mk for a release or the next snapshot.

Modes:
  release [VERSION] [--revision N]
      Remove the version suffix (1.5.7-SNAPSHOT -> 1.5.7, 1.5.7-rc.1 -> 1.5.7).

  snapshot [VERSION] [--revision N]
      Add the -SNAPSHOT suffix. Increments the patch version after a release
      (1.5.7 -> 1.5.8-SNAPSHOT) and keeps it after a pre-release
      (2.0.0-alpha.1 -> 2.0.0-SNAPSHOT).

VERSION (MAJOR.MINOR.PATCH) overrides the computed version, e.g. "snapshot 1.6.0".
In release mode it may include a pre-release suffix, e.g. "release 2.0.0-alpha.1".
The new version must be newer than the current one. Pre-releases of the same
version are ordered by SemVer rules (alpha.1 < alpha.2 < beta.1 < rc.1).

--revision sets COLLECTOR_REVISION. It defaults to 1 when the version changes.
To repackage the current version, pass it explicitly with a higher revision,
e.g. "release 1.5.7 --revision 2".
"""

import argparse
import dataclasses
import os
import re
import shutil
import sys
import tempfile
from pathlib import Path
from typing import Optional

VERSION_FILE = Path(__file__).resolve().parents[2] / "version.mk"
SNAPSHOT_SUFFIX = "-SNAPSHOT"
KEYS = ("MAJOR", "MINOR", "PATCH")


@dataclasses.dataclass(frozen=True)
class Version:
    core: tuple[int, int, int]
    suffix: str = ""
    revision: int = 1

    def __str__(self) -> str:
        return ".".join(str(part) for part in self.core) + self.suffix

    def describe(self) -> str:
        return f"{self} (revision {self.revision})"


def line_pattern(name: str) -> re.Pattern[str]:
    return re.compile(rf"^({re.escape(name)}[ \t]*=)[ \t]*(.*?)[ \t]*(?=\r?$)", re.MULTILINE)


def find_line(content: str, name: str) -> re.Match[str]:
    matches = list(line_pattern(name).finditer(content))
    if len(matches) != 1:
        sys.exit(
            f"ERROR: expected exactly one {name} in {VERSION_FILE}, found {len(matches)}")
    return matches[0]


def read_number(content: str, name: str) -> int:
    value = find_line(content, name).group(2)
    if not re.fullmatch(r"\d+", value):
        sys.exit(
            f"ERROR: invalid {name} value '{value}' in {VERSION_FILE}, expected a number")
    return int(value)


def read_version(content: str) -> Version:
    major, minor, patch = (read_number(
        content, f"COLLECTOR_VERSION_{key}") for key in KEYS)
    suffix = find_line(content, "COLLECTOR_VERSION_SUFFIX").group(2)
    revision = read_number(content, "COLLECTOR_REVISION")
    return Version((major, minor, patch), suffix, revision)


def replace_value(content: str, name: str, value: str) -> str:
    match = find_line(content, name)
    return content[:match.end(1)] + (f" {value}" if value else "") + content[match.end():]


def write_version(content: str, version: Version) -> None:
    for key, value in zip(KEYS, version.core):
        content = replace_value(
            content, f"COLLECTOR_VERSION_{key}", str(value))
    content = replace_value(content, "COLLECTOR_VERSION_SUFFIX", version.suffix)
    content = replace_value(content, "COLLECTOR_REVISION", str(version.revision))

    # Write to a temporary file and rename it to never leave a truncated version.mk behind.
    fd, tmp_path = tempfile.mkstemp(
        dir=VERSION_FILE.parent, prefix=f".{VERSION_FILE.name}.")
    try:
        with os.fdopen(fd, "w", newline="") as f:
            f.write(content)
        shutil.copymode(VERSION_FILE, tmp_path)
        os.replace(tmp_path, VERSION_FILE)
    except BaseException:
        os.unlink(tmp_path)
        raise


def parse_version(value: str) -> Version:
    match = re.fullmatch(
        r"(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?", value)
    if not match:
        raise argparse.ArgumentTypeError(
            f"invalid version '{value}', expected MAJOR.MINOR.PATCH[-PRERELEASE]")
    major, minor, patch = (int(part) for part in match.groups()[:3])
    return Version((major, minor, patch), match.group(4) or "")


def parse_revision(value: str) -> int:
    if not re.fullmatch(r"\d+", value) or int(value) < 1:
        raise argparse.ArgumentTypeError(
            f"invalid revision '{value}', expected a positive integer")
    return int(value)


def prerelease_key(suffix: str) -> list[tuple[int, int, str]]:
    # SemVer precedence: numeric identifiers sort numerically and before alphanumeric ones.
    return [(0, int(part), "") if part.isdigit() else (1, 0, part)
            for part in suffix.removeprefix("-").split(".")]


def is_newer(new: Version, current: Version) -> bool:
    if new.core != current.core:
        return new.core > current.core
    if new.suffix == current.suffix:
        # Repackaging the same version.
        return new.revision > current.revision
    if not new.suffix or not current.suffix:
        # Releasing a snapshot or pre-release of the same version, never the other way around.
        return not new.suffix
    if SNAPSHOT_SUFFIX in (new.suffix, current.suffix):
        # Snapshots sit between the pre-releases of the same version.
        return True
    return prerelease_key(new.suffix) > prerelease_key(current.suffix)


def release_target(current: Version, explicit: Optional[Version]) -> Version:
    if explicit:
        if explicit.suffix == SNAPSHOT_SUFFIX:
            sys.exit("ERROR: use snapshot mode to set a snapshot version")
        return explicit
    if not current.suffix:
        sys.exit(f"ERROR: current version {current} is already a release")
    return Version(current.core)


def snapshot_target(current: Version, explicit: Optional[Version]) -> Version:
    if explicit:
        if explicit.suffix:
            sys.exit(
                "ERROR: snapshot mode adds the -SNAPSHOT suffix, pass VERSION without a suffix")
        return Version(explicit.core, SNAPSHOT_SUFFIX)
    if current.suffix == SNAPSHOT_SUFFIX:
        sys.exit(f"ERROR: current version {current} is already a snapshot")
    if current.suffix:
        # Snapshot of the upcoming release after a pre-release.
        return Version(current.core, SNAPSHOT_SUFFIX)
    major, minor, patch = current.core
    return Version((major, minor, patch + 1), SNAPSHOT_SUFFIX)


def main() -> None:
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("mode", choices=("release", "snapshot"))
    parser.add_argument("version", nargs="?", type=parse_version,
                        help="explicit version (MAJOR.MINOR.PATCH[-PRERELEASE])")
    parser.add_argument("--revision", type=parse_revision, default=1,
                        help="package revision (default: 1)")
    args = parser.parse_args()

    with open(VERSION_FILE, newline="") as f:
        content = f.read()
    current = read_version(content)

    target = release_target if args.mode == "release" else snapshot_target
    new = dataclasses.replace(target(current, args.version), revision=args.revision)

    if not is_newer(new, current):
        sys.exit(f"ERROR: {new.describe()} is not newer than {current.describe()}")

    write_version(content, new)
    print(f"Changed version from {current.describe()} to {new.describe()}")


if __name__ == "__main__":
    main()
