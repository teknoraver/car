#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <dirent.h>
#include <string.h>
#include <fcntl.h>
#include <limits.h>
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

static int store_dir(char *file, int outfd)
{
	printf("Dir: %s\n", file);

	return 0;
}

static int store_file(char *file, int outfd)
{
	printf("File: %s\n", file);
	struct stat file_stat;
	struct entry entry = {
		.type = FILE_TYPE,
		.namelen = strlen(file),
	};
	size_t pos;

	if (stat(file, &file_stat) == -1) {
		perror("stat");
		return -1;
	}

	entry.datasize = file_stat.st_size;

	pos = lseek(outfd, 0, SEEK_CUR);
	pos += sizeof(entry) + entry.namelen + 1;
	if (pos % COW_ALIGNMENT)
		entry.padding = COW_ALIGNMENT - (pos % COW_ALIGNMENT);

	write(outfd, &entry, sizeof(entry));
	write(outfd, file, entry.namelen + 1);

	if (entry.padding)
		lseek(outfd, entry.padding, SEEK_CUR);

	copy_data(file, outfd);

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

		if (stat(path, &entry_stat) == -1) {
			perror("stat");
			return -1;
		}

		if (S_ISREG(entry_stat.st_mode))
			store_file(path, outfd);
		else if (S_ISDIR(entry_stat.st_mode))
			compress2(path, outfd);
	}

	closedir(dir);

	return 0;
}

int compress(char *inputdir, char *outputfile)
{
	int outfd;

	outfd = open(outputfile, O_WRONLY | O_CREAT | O_TRUNC, 0666);
	if (!outfd) {
		perror("open");
		return -1;
	}

	return compress2(inputdir, outfd);
}
