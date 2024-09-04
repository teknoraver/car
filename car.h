#pragma once

#include <stddef.h>
#include <stdbool.h>
#include <stdint.h>

#define FILE_TYPE	1
#define DIR_TYPE	2

#define COW_ALIGNMENT	4096

extern bool verbose;

struct entry {
	uint8_t type;
	uint32_t namelen;
	uint32_t padding;
	uint64_t datasize;
	char name[];
};

int compress(char *inputdir, char *outputfile);
int extract(char *inputfile, char *outputdir);
