CFLAGS := -O2 -pipe -Wall

car: car.o compress.o extract.o

clean::
	$(RM) *.o car
