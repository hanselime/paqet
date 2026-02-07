package conf

import (
	"fmt"
	"os"
	"paqet/internal/flog"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
)

type ServerConfig struct {
	Server    Server    `yaml:"server"`
	SOCKS5    []SOCKS5  `yaml:"socks5"`
	Forward   []Forward `yaml:"forward"`
	Transport Transport `yaml:"transport"`
}

type Conf struct {
	Role      string         `yaml:"role"`
	Log       Log            `yaml:"log"`
	Listen    Server         `yaml:"listen"`
	SOCKS5    []SOCKS5       `yaml:"socks5"`
	Forward   []Forward      `yaml:"forward"`
	Network   Network        `yaml:"network"`
	Server    Server         `yaml:"server"`
	Transport Transport      `yaml:"transport"`
	Servers   []ServerConfig `yaml:"servers"`
}

func LoadFromFile(path string) (*Conf, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var conf Conf

	if err := yaml.Unmarshal(data, &conf); err != nil {
		return &conf, err
	}

	validRoles := []string{"client", "server"}
	if !slices.Contains(validRoles, conf.Role) {
		return nil, fmt.Errorf("role must be 'client' or 'server'")
	}

	conf.setDefaults()
	if err := conf.validate(); err != nil {
		return &conf, err
	}

	return &conf, nil
}

func (c *Conf) setDefaults() {
	c.Log.setDefaults()
	c.Listen.setDefaults()
	c.Network.setDefaults(c.Role)

	if c.Role == "client" {
		if len(c.Servers) == 0 {
			c.Servers = append(c.Servers, ServerConfig{
				Server:    c.Server,
				SOCKS5:    c.SOCKS5,
				Forward:   c.Forward,
				Transport: c.Transport,
			})
		}
		for i := range c.Servers {
			c.Servers[i].Server.setDefaults()
			for j := range c.Servers[i].SOCKS5 {
				c.Servers[i].SOCKS5[j].setDefaults()
			}
			for j := range c.Servers[i].Forward {
				c.Servers[i].Forward[j].setDefaults()
			}
			c.Servers[i].Transport.setDefaults(c.Role)
		}
	}

	c.Transport.setDefaults(c.Role)
}

func (c *Conf) validate() error {
	var allErrors []error

	allErrors = append(allErrors, c.Log.validate()...)
	allErrors = append(allErrors, c.Network.validate()...)

	if c.Role == "server" {
		allErrors = append(allErrors, c.Listen.validate()...)
		allErrors = append(allErrors, c.Transport.validate()...)
	} else {
		if len(c.Servers) == 0 {
			allErrors = append(allErrors, fmt.Errorf("no servers configured"))
		}

		usedAddrs := make(map[string]string)

		for i := range c.Servers {
			srv := &c.Servers[i]
			if len(srv.SOCKS5) == 0 && len(srv.Forward) == 0 {
				flog.Warnf("warning: server[%d] configured but no SOCKS5 or forward rules found", i)
			}

			allErrors = append(allErrors, srv.Server.validate()...)
			allErrors = append(allErrors, srv.Transport.validate()...)

			for j := range srv.SOCKS5 {
				errs := srv.SOCKS5[j].validate()
				for _, err := range errs {
					allErrors = append(allErrors, fmt.Errorf("server[%d].socks5[%d] %v", i, j, err))
				}
				addr := fmt.Sprint(srv.SOCKS5[j].Listen)
				if owner, ok := usedAddrs[addr]; ok {
					allErrors = append(allErrors, fmt.Errorf("listen address collision: '%s' is used by %s and server[%d].socks5[%d]", addr, owner, i, j))
				} else {
					usedAddrs[addr] = fmt.Sprintf("server[%d].socks5[%d]", i, j)
				}
			}
			for j := range srv.Forward {
				errs := srv.Forward[j].validate()
				for _, err := range errs {
					allErrors = append(allErrors, fmt.Errorf("server[%d].forward[%d] %v", i, j, err))
				}
				addr := fmt.Sprint(srv.Forward[j].Listen)
				if owner, ok := usedAddrs[addr]; ok {
					allErrors = append(allErrors, fmt.Errorf("listen address collision: '%s' is used by %s and server[%d].forward[%d]", addr, owner, i, j))
				} else {
					usedAddrs[addr] = fmt.Sprintf("server[%d].forward[%d]", i, j)
				}
			}

			if srv.Server.Addr != nil {
				if srv.Server.Addr.IP.To4() != nil && c.Network.IPv4.Addr == nil {
					allErrors = append(allErrors, fmt.Errorf("server[%d] address is IPv4, but the IPv4 interface is not configured", i))
				}
				if srv.Server.Addr.IP.To4() == nil && c.Network.IPv6.Addr == nil {
					allErrors = append(allErrors, fmt.Errorf("server[%d] address is IPv6, but the IPv6 interface is not configured", i))
				}
			}
			if srv.Transport.Conn > 1 && c.Network.Port != 0 {
				allErrors = append(allErrors, fmt.Errorf("only one connection is allowed when a client port is explicitly set"))
			}
		}
	}
	return writeErr(allErrors)
}

func writeErr(allErrors []error) error {
	if len(allErrors) > 0 {
		var messages []string
		for _, err := range allErrors {
			messages = append(messages, err.Error())
		}
		return fmt.Errorf("validation failed:\n  - %s", strings.Join(messages, "\n  - "))
	}
	return nil
}
