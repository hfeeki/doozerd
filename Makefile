# figure out what GOROOT is supposed to be
GOROOT ?= $(shell printf 't:;@echo $$(GOROOT)\n' | gomake -f -)
include $(GOROOT)/src/Make.inc

TARG=logfs
GOFILES=\
	logfs.go\

include $(GOROOT)/src/Make.pkg
