# CAR - Copy-on-write Archive
`car` is a tool to create and extract archives without copying data. This is possible by using the filesystem capabilities to reflink data from and to the archive.
A filesystem with [reflink](https://man7.org/linux/man-pages/man2/ioctl_ficlone.2.html) support is needed, like [BtrFS](https://docs.kernel.org/filesystems/btrfs.html) or [XFS](https://xfs.wiki.kernel.org/).
## Usage
`car` mimics the syntax of the glorious `tar` tool, so to create an archive, we do:
```
$ car -c -v -f dir.car dir
dir
dir/a_200
dir/b_4k
dir/c_4k10
dir/link
```
While to extract it:
```
$ car -x -v -f dir.car
dir/a_200
dir/b_4k
dir/c_4k10
dir/link
```
## Benchmark
The following benchmark was done on a BtrFS filesystem with a 6.10 aarch64 kernel.

Test setup:
```
$ ll software/
total 1.2G
-rw-r--r--. 1 teknoraver teknoraver 631M Aug 31 14:07 debian-12.7.0-amd64-netinst.iso
-rw-r--r--. 1 teknoraver teknoraver 154M Aug  1 10:51 gcc-14.2.0.tar.gz
-rw-r--r--. 1 teknoraver teknoraver  18M Jul 22 13:59 glibc-2.40.tar.xz
-rw-r--r--. 1 teknoraver teknoraver  27M Sep  5 17:20 go1.23.1.src.tar.gz
-rw-r--r--. 1 teknoraver teknoraver 139M Sep  8 08:08 linux-6.10.9.tar.xz
-rw-r--r--. 1 teknoraver teknoraver 207M Sep  5 18:05 rustc-1.81.0-src.tar.xz
```
Archive creation:
```
$ time tar cf software.tar software/

real    0m1.936s
user    0m0.020s
sys     0m0.600s

$ time car -c -f software.car software/

real    0m0.052s
user    0m0.000s
sys     0m0.014s

$ ll software.*
-rw-r--r--. 1 teknoraver teknoraver 1.2G Sep  8 16:08 software.car
-rw-r--r--. 1 teknoraver teknoraver 1.2G Sep  8 16:08 software.tar
```
Archive extraction:
```
$ time tar xf software.tar

real    0m2.394s
user    0m0.026s
sys     0m0.668s

$ car -x -f software.car

real    0m0.059s
user    0m0.001s
sys     0m0.015s
```
perf stat:
```
$ perf stat tar xf software.tar

 Performance counter stats for 'tar xf software.tar':

            691.06 msec task-clock:u                     #    0.765 CPUs utilized
                 0      context-switches:u               #    0.000 /sec
                 0      cpu-migrations:u                 #    0.000 /sec
               187      page-faults:u                    #  270.599 /sec
   <not supported>      cycles:u
   <not supported>      instructions:u
   <not supported>      branches:u
   <not supported>      branch-misses:u

       0.902759486 seconds time elapsed

       0.015161000 seconds user
       0.624258000 seconds sys

$ perf stat car -x -f software.car

 Performance counter stats for 'car -x -f software.car':

              6.22 msec task-clock:u                     #    0.248 CPUs utilized
                 0      context-switches:u               #    0.000 /sec
                 0      cpu-migrations:u                 #    0.000 /sec
               210      page-faults:u                    #   33.754 K/sec
   <not supported>      cycles:u
   <not supported>      instructions:u
   <not supported>      branches:u
   <not supported>      branch-misses:u

       0.025132295 seconds time elapsed

       0.001251000 seconds user
       0.004706000 seconds sys
```
## File format
The archive is composed by a list of entries, each one representing a file.
An entry is composed by a list of [TLV (Type-Length-Value)](https://en.wikipedia.org/wiki/Type%E2%80%93length%E2%80%93value) fields, where the type is a 2 byte word, the length is a 2 byte word and the value is a variable length field.
The list of types are:
1. Header (0x0001)  
*Mandatory*, contains the following fields:
* uint32 file mode, which in turn contains the file type in the first bits and the permissions in the lowest 12 bits.
* uint32 User ID
* uint32 Group ID
* uint64 Modification time (nanoseconds since epoch)
2. Name (0x0002)  
*Mandatory*, contains the name of the file, as a string.
3. Data (0x0003)  
*Mandatory*, must be the last field. If the file is not a regular file or is empty, the tag lenght is 0, otherwise it contains the following fields:
* uint64 the file size
* uint32 number of padding bytes between the tag and the file content
4. Link target (0x0004)  
Contains the target of a symlink, as a string.
5. Device (0x0005)  
uint32 contains the major and minor numbers, Mandatory for block and character devices.

After the last tag, which must be 'Data', there is the padding and the file content.  
After the last entry there is the magic written in backwards (`!RAC`) to signal the end of the archive. The archive must be padded with zero bytes so that its length is a multiple of 4k. This is required otherwise the reflink operation will fail when extracting the last entries.
