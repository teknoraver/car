#pragma once

#include <stddef.h>
#include <stdint.h>

#define FILE_TYPE	1
#define DIR_TYPE	2

#define COW_ALIGNMENT	4096

struct entry {
	uint8_t type;
	uint32_t namelen;
	uint16_t padding;
	uint64_t datasize;
	char name[];
};

int compress(char *inputdir, char *outputfile);
int extract(char *inputfile, char *outputdir);
