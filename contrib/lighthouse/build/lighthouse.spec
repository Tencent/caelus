Name: lighthouse
Version: %{version}
Release: %{commit}%{?dist}
Summary: lighthouse plugin

Group: Development/GAIA
License: MIT
Source: lighthouse.tar.gz

Requires: systemd-units

%define pkgname  %{name}-%{version}-%{release}

%description
lighthouse plugin

%prep
%setup -n lighthouse-%{version}

%build
make binary

%install
install -d $RPM_BUILD_ROOT/%{_bindir}
install -d $RPM_BUILD_ROOT/%{_unitdir}
install -d $RPM_BUILD_ROOT/etc/lighthouse

install -p -m 755 ./_output/bin/lighthouse $RPM_BUILD_ROOT/%{_bindir}/lighthouse
install -p -m 644 ./build/config $RPM_BUILD_ROOT/etc/lighthouse/config
install -p -m 644 ./build/config.yaml $RPM_BUILD_ROOT/etc/lighthouse/config.yaml
install -p -m 644 ./build/lighthouse.service $RPM_BUILD_ROOT/%{_unitdir}/

%clean
rm -rf $RPM_BUILD_ROOT

%files
%config(noreplace,missingok) /etc/lighthouse/config
%config(noreplace,missingok) /etc/lighthouse/config.yaml

/%{_unitdir}/lighthouse.service

/%{_bindir}/lighthouse
