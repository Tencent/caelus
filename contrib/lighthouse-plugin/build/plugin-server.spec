Name: plugin-server
Version: %{version}
Release: %{commit}%{?dist}
Summary: lighthouse plugin

Group: Development/GAIA
License: MIT
Source: plugin-server.tar.gz

Requires: systemd-units

%define pkgname  %{name}-%{version}-%{release}

%description
lighthouse plugin

%prep
%setup -n plugin-server-%{version}

%build
make binary

%install
install -d $RPM_BUILD_ROOT/%{_bindir}
install -d $RPM_BUILD_ROOT/%{_unitdir}
install -d $RPM_BUILD_ROOT/etc/plugin-server

install -p -m 755 ./_output/bin/plugin-server $RPM_BUILD_ROOT/%{_bindir}/plugin-server
install -p -m 644 ./build/config $RPM_BUILD_ROOT/etc/plugin-server/config
install -p -m 644 ./build/plugin-server.service $RPM_BUILD_ROOT/%{_unitdir}/

%clean
rm -rf $RPM_BUILD_ROOT

%files
%config(noreplace,missingok) /etc/plugin-server/config

/%{_unitdir}/plugin-server.service

/%{_bindir}/plugin-server
