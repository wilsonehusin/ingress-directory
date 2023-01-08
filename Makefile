TAG?=latest

include Makefile.bindl

serve/app.css: bin/tailwindcss
	bin/tailwindcss \
		--input serve/base.css \
		--output serve/app.css \
		--minify

.PHONY: ko/publish
ko/publish: bin/ko serve/app.css
	bin/ko build --base-import-paths

.PHONY: ko/release
ko/release: bin/ko serve/app.css
	bin/ko build --base-import-paths --tags ${TAG}
