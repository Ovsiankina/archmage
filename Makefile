BINDIR ?= /bin

archmage: $(wildcard *.go) go.mod go.sum
	go build -o $@ .

install: archmage
	install -Dm755 archmage $(DESTDIR)$(BINDIR)/archmage

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/archmage

test:
	go test ./...

clean:
	rm -f archmage

.PHONY: install uninstall test clean
