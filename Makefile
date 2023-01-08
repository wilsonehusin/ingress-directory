TAG?=latest

include Makefile.bindl

serve/app.css: bin/tailwindcss
	bin/tailwindcss \
		--input serve/base.css \
		--output serve/app.css \
		--minify

.PHONY: ko/auth
ko/auth: bin/ko
	# username doesn't seem to matter
	echo "${{ github.token }}" | bin/ko login ghcr.io \
		--username "dummy" --password-stdin

.PHONY: ko/publish
ko/publish: bin/ko serve/app.css
	bin/ko build --base-import-paths

.PHONY: ko/release
ko/release: bin/ko serve/app.css
	bin/ko build --base-import-paths --tags ${TAG}
