#include <stdio.h>
#include <stdlib.h>
#include <stdbool.h>
#include <unistd.h>

#include "car.h"

bool verbose = false;

static void usage(void)
{
	fprintf(stderr, "Usage: [-h] [-c] [-x] [file...]\n");
	exit(1);
}

int main(int argc, char *argv[])
{
	int c;
	bool comp = false, extr = false;
	char *file = NULL;

	while ((c = getopt(argc, argv, "hcx:f:v")) != -1) {
		switch(c) {
		case 'h':
			usage();
			break;
		case 'c':
			comp = true;
			break;
		case 'x':
			extr = true;
			break;
		case 'f':
			file = optarg;
			break;
		case 'v':
			verbose = true;
			break;
		case '?':
		default:
			usage();
		}
	}

	if (!comp && !extr) {
		fprintf(stderr, "Error: must specify either -c or -x\n");
		return 1;
	}

	if (comp && extr) {
		fprintf(stderr, "Error: cannot specify both -c and -x\n");
		return 1;
	}

	if (optind == argc) {
		fprintf(stderr, "Error: must specify at least one file\n");
		return 1;
	}

	if (comp) {
		printf("Compressing %s to %s\n", argv[optind], file);
		compress(argv[optind], file);
	} else {
		printf("Extracting %s to %s\n", file, argv[optind]);
		extract(file, argv[optind]);
	}
}
