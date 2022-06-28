#!/bin/bash

set -euo pipefail
set -x

SCRIPT_PATH=$(dirname -- "$( readlink -f -- "$0"; )")
INIT_SCRIPT="${SCRIPT_PATH}/runcon-init.sh"

IMG_PATH=$1


# clean up
cleanup() {
    echo "Clean up"
    rm -rf ${WORKROOT}
}

# is_array
is_array() {
  eval [[ "\"\${!$1[*]}\"" =~ "[1-9]" ]]
}

# vars
TMPDIR=${TMPDIR-/tmp}
WORKROOT=${TMPDIR}/$(uuidgen)
FAKEROOT=${WORKROOT}/root
CONTAINER_TAR=${WORKROOT}/container_fs.tar
CONTAINER_CFG=${WORKROOT}/container_cfg.json

# setup
echo "Create work root ${WORKROOT}"
mkdir -p ${WORKROOT}
trap cleanup EXIT

# container config
crane config ${IMG_PATH} | jq .container_config > ${CONTAINER_CFG}
CONTAINER_ENV=()
while read line; do
    CONTAINER_ENV+=("$line")
done < <(jq -r .Env[] ${CONTAINER_CFG})

# CONTAINER_CMD=()
# while read line; do
#     CONTAINER_CMD+=("$line")
# done < <(jq -r .Cmd[] ${CONTAINER_CFG}) # array?
# for ev in ${CONTAINER_CMD[@]}; do
#     echo "CMD ${ev}"
# done

# CONTAINER_ENTRYPOINT=()
# while read line; do
#     CONTAINER_ENTRYPOINT+=("$line")
# done < <(jq -r .Entrypoint[] ${CONTAINER_CFG}) # array?
# for ev in ${CONTAINER_ENTRYPOINT[@]}; do
#     echo "Entry ${ev}"
# done

CONTAINER_SHELL=$(jq -r .Shell ${CONTAINER_CFG}) # array?
if [ -z "${CONTAINER_SHELL}" -o "${CONTAINER_SHELL}" = "null" ]; then 
    CONTAINER_SHELL="/bin/sh"
fi
echo "SHELL ${CONTAINER_SHELL}"

CONTAINER_WDIR=$(jq -r .WorkingDir ${CONTAINER_CFG})
if [ -z "${CONTAINER_WDIR}" -o "${CONTAINER_WDIR}" = "null" ]; then 
    CONTAINER_WDIR="/"
fi
echo "WDIR ${CONTAINER_WDIR}"


# download container
crane export ${IMG_PATH} ${CONTAINER_TAR}
mkdir -p ${FAKEROOT}
chmod u+w ${FAKEROOT} -R
cd ${FAKEROOT}
tar xvf ${CONTAINER_TAR} --delay-directory-restore

# create namespace (user, mount, pid with fork, map to root user)
unshare -Urmpf --mount-proc ${INIT_SCRIPT} ${FAKEROOT} ${CONTAINER_WDIR}

# # pivot root
# mount --bind ${FAKEROOT} ${FAKEROOT}
# mkdir -p ${FAKEROOT}/.old_root
# pivot_root ${FAKEROOT} ${FAKEROOT}/.old_root

# # # run bind mounts to old root
# # mkdir -p ${BIND_PATH}
# # mount --bind /.old_root${BIND_PATH} ${BIND_PATH} 
# # # only for read-only mounts
# # # mount -o remount,ro.bind ${BIND_PATH}


# # # mount tmpfs
# # mkdir -p ${TMPFS_PATH}
# # # -o size is optional, TMPFS_NAME can be none
# # mount -t tmpfs -o size=${TMPFS_SIZE} none ${TMPFS_PATH}

# # remove old_root
# umount /.old_root
# rmdir /.old_root

# # run command in container
# cd ${CONTAINER_WDIR}

# # env setup
# for ev in ${CONTAINER_ENV[@]}; do
#     set ${ev}
# done

# # cmd (fake it)
# /bin/sh
