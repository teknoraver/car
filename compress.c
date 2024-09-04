#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <dirent.h>
#include <string.h>
#include <fcntl.h>
#include <limits.h>
#include <sys/ioctl.h>
#include <linux/fs.h>
#include <sys/stat.h>

#include "car.h"

static int copy_data(char *file, int outfd)
{
	int infd;
	char buf[4096];
	ssize_t n;

	infd = open(file, O_RDONLY);
	if (infd == -1) {
		perror("open");
		return -1;
	}

	while ((n = read(infd, buf, sizeof(buf))) > 0)
		write(outfd, buf, n);

	close(infd);

	return 0;
}

static int reflink_data(char *file, int outfd, uint64_t len, uint64_t off)
{
	struct file_clone_range fcr = {
		.dest_offset = off,
	};
	int infd;
	int ret;
	int leftover;

	if (len < COW_ALIGNMENT)
		return copy_data(file, outfd);

	leftover = len % COW_ALIGNMENT;
	fcr.src_length = len - leftover;

	infd = open(file, O_RDONLY);
	if (infd == -1) {
		perror("open");
		return -1;
	}
	fcr.src_fd = infd;

	ret = ioctl(outfd, FICLONERANGE, &fcr);
	if (ret == -1) {
		perror("ioctl(FICLONERANGE)");
		return copy_data(file, outfd);
	}

	if (leftover) {
		char buf[COW_ALIGNMENT];
		lseek(outfd, 0, SEEK_END);
		lseek(infd, fcr.src_length, SEEK_SET);
		read(infd, buf, leftover);
		write(outfd, buf, leftover);
	}

	close(infd);

	return 0;
}

static int store_dir(char *file, int outfd)
{
	if (verbose)
		printf("Dir: %s\n", file);

	return 0;
}

static int store_file(char *file, int outfd, size_t datasize)
{
	if (verbose)
		printf("File: %s\n", file);
	struct entry entry = {
		.type = FILE_TYPE,
		.namelen = strlen(file),
	};
	size_t pos;

	entry.datasize = datasize;

	pos = lseek(outfd, 0, SEEK_CUR);
	pos += sizeof(entry) + entry.namelen + 1;
	if (pos % COW_ALIGNMENT)
		entry.padding = COW_ALIGNMENT - (pos % COW_ALIGNMENT);

	write(outfd, &entry, sizeof(entry));
	write(outfd, file, entry.namelen + 1);

	if (entry.padding)
		lseek(outfd, entry.padding, SEEK_CUR);

	reflink_data(file, outfd, entry.datasize, pos + entry.padding);

	return 0;
}

static int compress2(char *inputdir, int outfd)
{
	DIR *dir;
	struct dirent *entry;

	dir = opendir(inputdir);
	if (!dir) {
		perror("open");
		return -1;
	}
	store_dir(inputdir, outfd);

	while ((entry = readdir(dir)) != NULL) {
		char path[PATH_MAX];
		struct stat entry_stat;

		if (strcmp(entry->d_name, ".") == 0 || strcmp(entry->d_name, "..") == 0)
			continue;

		snprintf(path, sizeof(path), "%s/%s", inputdir, entry->d_name);

		if (lstat(path, &entry_stat) == -1) {
			perror("stat");
			return -1;
		}

		if (S_ISREG(entry_stat.st_mode))
			store_file(path, outfd, entry_stat.st_size);
		else if (S_ISDIR(entry_stat.st_mode))
			compress2(path, outfd);
	}

	closedir(dir);

	return 0;
}

int compress(char *inputdir, char *outputfile)
{
	int outfd;

	outfd = creat(outputfile, 0666);
	if (!outfd) {
		perror("open");
		return -1;
	}

	return compress2(inputdir, outfd);
}
