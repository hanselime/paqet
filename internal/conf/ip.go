package conf

type IP struct {
	Protocol int `yaml:"protocol"`
}

func (i *IP) setDefaults() {
	if i.Protocol == 0 {
		i.Protocol = 6
	}
}
