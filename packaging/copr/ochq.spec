# Fedora COPR package for the ochq client, built from source:
#   sudo dnf copr enable mukhiddinov/ochq && sudo dnf install ochq
# packaging/copr/srpm.sh fills in the version and bundles the Go modules
# (vendor/), because COPR builds have no network access.

# Go binaries are statically linked and stripped (-s -w): no debuginfo.
%global debug_package %{nil}

Name:           ochq
Version:        @VERSION@
Release:        1%{?dist}
Summary:        Expose a local port to the internet through an OCHQ tunnel

License:        MIT
URL:            https://github.com/mukhiddinov/ochq
Source0:        %{name}-%{version}.tar.gz

ExclusiveArch:  x86_64 aarch64
BuildRequires:  golang >= 1.26

%description
ochq gives the service on your localhost a public HTTPS URL, such as
https://my-app.ochq.uz, through an encrypted tunnel. No account, router
setup or config file: run "ochq 3000" and share the link.

%prep
%autosetup

%build
export CGO_ENABLED=0 GOFLAGS=-mod=vendor GOTOOLCHAIN=local GOPROXY=off
go build -trimpath \
    -ldflags "-s -w -X github.com/mukhiddinov/ochq/pkg/version.Version=%{version}" \
    -o bin/ochq ./cmd/ochq
for shell in bash zsh fish; do
    bin/ochq completion "$shell" > "ochq.$shell"
done

%check
export CGO_ENABLED=0 GOFLAGS=-mod=vendor GOTOOLCHAIN=local GOPROXY=off
go test ./...
bin/ochq version | grep -qx "ochq %{version}"

%install
install -Dpm 0755 bin/ochq %{buildroot}%{_bindir}/ochq
install -Dpm 0644 ochq.bash %{buildroot}%{_datadir}/bash-completion/completions/ochq
install -Dpm 0644 ochq.zsh %{buildroot}%{_datadir}/zsh/site-functions/_ochq
install -Dpm 0644 ochq.fish %{buildroot}%{_datadir}/fish/vendor_completions.d/ochq.fish

%files
%license LICENSE
%doc README.md
%{_bindir}/ochq
%{_datadir}/bash-completion/completions/ochq
%dir %{_datadir}/zsh
%dir %{_datadir}/zsh/site-functions
%{_datadir}/zsh/site-functions/_ochq
%dir %{_datadir}/fish
%dir %{_datadir}/fish/vendor_completions.d
%{_datadir}/fish/vendor_completions.d/ochq.fish

%changelog
* Sat Oct 24 2026 Mukhiddinov <muxiddinovxusniddin34@gmail.com> - 0.3.1-1
- Package ochq for Fedora COPR; release notes live on GitHub:
  https://github.com/mukhiddinov/ochq/releases
