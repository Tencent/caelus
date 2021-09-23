#!/bin/bash

# -----------------------------------------------------------------------------
# Version management helpers.  These functions help to set, save and load the
# following variables:
#
#    GIT_COMMIT - The git commit id corresponding to this
#          source code.
#    GIT_TREE_STATE - "clean" indicates no changes since the git commit id
#        "dirty" indicates source code changes after the git commit id
#        "archive" indicates the tree was produced by 'git archive'
#    GIT_VERSION - "vX.Y" used to indicate the last release version.
#    GIT_MAJOR - The major part of the version
#    GIT_MINOR - The minor component of the version

# Grovels through git to set a set of env variables.
#
# If GIT_VERSION_FILE, this function will load from that file instead of
# querying git.
api::version::get_version_vars() {
  if [[ -n ${GIT_VERSION_FILE-} ]] && [[ -f ${GIT_VERSION_FILE-} ]]; then
    api::version::load_version_vars "${GIT_VERSION_FILE}"
    return
  fi

  # If the caelus source was exported through git archive, then
  # we likely don't have a git tree, but these magic values may be filled in.
  if [[ '$Format:%%$' == "%" ]]; then
    GIT_COMMIT='$Format:%H$'
    GIT_TREE_STATE="archive"
    # When a 'git archive' is exported, the '$Format:%D$' below will look
    # something like 'HEAD -> release-1.8, tag: v1.8.3' where then 'tag: '
    # can be extracted from it.
    if [[ '$Format:%D$' =~ tag:\ (v[^ ]+) ]]; then
     GIT_VERSION="${BASH_REMATCH[1]}"
    fi
  fi

  local git=(git --work-tree "${BASE_DIR}")

  if [[ -n ${GIT_COMMIT-} ]] || GIT_COMMIT=$("${git[@]}" rev-parse "HEAD^{commit}" 2>/dev/null); then
    if [[ -z ${GIT_TREE_STATE-} ]]; then
      # Check if the tree is dirty.  default to dirty
      if git_status=$("${git[@]}" status --porcelain 2>/dev/null) && [[ -z ${git_status} ]]; then
        GIT_TREE_STATE="clean"
      else
        GIT_TREE_STATE="dirty"
      fi
    fi

    # Use git branch to find the branch of current commit.
    if [[ -z ${GIT_BRANCH-} ]] ; then
      GIT_BRANCH=$("${git[@]}" branch | sed -n -e 's/^\* \(.*\)/\1/p' | tr -d "[:blank:]" 2>/dev/null)
    fi

    # Use git describe to find the version based on annotated tags.
    if [[ -n ${GIT_VERSION-} ]] || GIT_VERSION=$("${git[@]}" describe --tags --abbrev=14 "${GIT_COMMIT}^{commit}" 2>/dev/null); then
      # This translates the "git describe" to an actual semver.org
      # compatible semantic version that looks something like this:
      #   v1.1.0-alpha.0.6+84c76d1142ea4d
      #
      # TODO: We continue calling this "git version" because so many
      # downstream consumers are expecting it there.
      DASHES_IN_VERSION=$(echo "${GIT_VERSION}" | sed "s/[^-]//g")
      if [[ "${DASHES_IN_VERSION}" == "---" ]] ; then
        # We have distance to subversion (v1.1.0-subversion-1-gCommitHash)
        GIT_VERSION=$(echo "${GIT_VERSION}" | sed "s/-\([0-9]\{1,\}\)-g\([0-9a-f]\{14\}\)$/.\1\+\2/")
      elif [[ "${DASHES_IN_VERSION}" == "--" ]] ; then
        # We have distance to base tag (v1.1.0-1-gCommitHash)
        GIT_VERSION=$(echo "${GIT_VERSION}" | sed "s/-g\([0-9a-f]\{14\}\)$/+\1/")
      fi
      if [[ "${GIT_TREE_STATE}" == "dirty" ]]; then
        # git describe --dirty only considers changes to existing files, but
        # that is problematic since new untracked .go files affect the build,
        # so use our idea of "dirty" from git status instead.
        GIT_VERSION+="-dirty"
      fi

      # Try to match the "git describe" output to a regex to try to extract
      # the "major" and "minor" versions and whether this is the exact tagged
      # version or whether the tree is between two tagged versions.
      if [[ "${GIT_VERSION}" =~ ^v([0-9]+)\.([0-9]+)(\.[0-9]+)?([-].*)?$ ]]; then
        GIT_MAJOR=${BASH_REMATCH[1]}
        GIT_MINOR=${BASH_REMATCH[2]}
        if [[ -n "${BASH_REMATCH[4]}" ]]; then
          GIT_MINOR+="+"
        fi
      fi
    fi
  fi
}

# Saves the environment flags to $1
api::version::save_version_vars() {
  local version_file=${1-}
  [[ -n ${version_file} ]] || {
    echo "!!! Internal error.  No file specified in api::version::save_version_vars"
    return 1
  }

  cat <<EOF >"${version_file}"
GIT_BRANCH='${GIT_BRANCH-}'
GIT_COMMIT='${GIT_COMMIT-}'
GIT_TREE_STATE='${GIT_TREE_STATE-}'
GIT_VERSION='${GIT_VERSION-}'
GIT_MAJOR='${GIT_MAJOR-}'
GIT_MINOR='${GIT_MINOR-}'
EOF
}

# Loads up the version variables from file $1
api::version::load_version_vars() {
  local version_file=${1-}
  [[ -n ${version_file} ]] || {
    echo "!!! Internal error.  No file specified in api::version::load_version_vars"
    return 1
  }

  source "${version_file}"
}

api::version::ldflag() {
  local key=${1}
  local val=${2}

  # If you update these, also update the list pkg/version/def.bzl.
  echo "-X ${PACKAGE}/pkg/version.${key}=${val}"
}

# Prints the value that needs to be passed to the -ldflags parameter of go build
# in order to set the Kubernetes based on the git tree status.
# IMPORTANT: if you update any of these, also update the lists in
# pkg/version/def.bzl and hack/print-workspace-status.sh.
api::version::ldflags() {
  api::version::get_version_vars

  local buildDate=
  [[ -z ${SOURCE_DATE_EPOCH-} ]] || buildDate="--date=@${SOURCE_DATE_EPOCH}"
  local -a ldflags=($(api::version::ldflag "buildDate" "$(date ${buildDate} -u +'%Y-%m-%dT%H:%M:%SZ')"))
  if [[ -n ${GIT_BRANCH-} ]]; then
    ldflags+=($(api::version::ldflag "gitBranch" "${GIT_BRANCH}"))
  fi

  if [[ -n ${GIT_COMMIT-} ]]; then
    ldflags+=($(api::version::ldflag "gitCommit" "${GIT_COMMIT}"))
    ldflags+=($(api::version::ldflag "gitTreeState" "${GIT_TREE_STATE}"))
  fi

  if [[ -n ${GIT_VERSION-} ]]; then
    ldflags+=($(api::version::ldflag "gitVersion" "${GIT_VERSION}"))
  fi

  if [[ -n ${GIT_MAJOR-} && -n ${GIT_MINOR-} ]]; then
    ldflags+=(
      $(api::version::ldflag "gitMajor" "${GIT_MAJOR}")
      $(api::version::ldflag "gitMinor" "${GIT_MINOR}")
    )
  fi

  # The -ldflags parameter takes a single string, so join the output.
  echo "${ldflags[*]-}"
}
