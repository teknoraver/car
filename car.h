#pragma once

#include <stddef.h>

enum file_type {
	FILE_TYPE,
	DIR_TYPE
};

struct entry {
	enum file_type type;
	size_t namelen;
	size_t datalen;
	char name[];
};

int compress(char *inputdir, char *outputfile);
int extract(char *inputfile, char *outputdir);
