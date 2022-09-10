#!/bin/bash

FAKEROOT=$1
CONTAINER_WDIR=$2

# pivot root
mount --bind ${FAKEROOT} ${FAKEROOT}
mkdir -p ${FAKEROOT}/.old_root
pivot_root ${FAKEROOT} ${FAKEROOT}/.old_root

# remove old_root
umount /.old_root
rmdir /.old_root

# run command in container
cd ${CONTAINER_WDIR}

# env setup
# for ev in ${CONTAINER_ENV[@]}; do
#     set ${ev}
# done

# cmd (fake it)
/bin/sh