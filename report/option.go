package report

import "golang.org/x/text/language"

type Options struct {
	Locale *language.Tag
	Title  string
}

type Option func(*Options)

func Locale(locale *language.Tag) Option {
	return func(args *Options) {
		args.Locale = locale
	}
}

func Title(title string) Option {
	return func(args *Options) {
		args.Title = title
	}
}
