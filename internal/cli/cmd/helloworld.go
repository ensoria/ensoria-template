/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>

*/

// ensoriaコマンドを簡単に追加できるようにするためのパッケージ。
// 開発に便利なコマンドを追加したり、バッチ処理のためのコマンドを追加する。

// さまざまなドメインをまたいだコマンドを実行したい場合は
// service adapterを介して、各ドメインのコマンドを実行するようにする。
// マイクロサービスにした際に、service間がgRPCになれば、データをストリーミングで
// やりとりできるので、大量のデータでも扱いやすいはず。

// 各モジュールやqueryの中にはコマンドは追加しない。
// moduleやqueryの中にコマンドがあると、いろいろバラけてややこしくなる。
// さらに、マイクロサービスにした際に、バッチ処理は、バッチ専用のPodに任せたほうがよさそう
// バッチをメインで処理するpod(central)が、各サービスからデータを取得したり保存したほうが、
// 各サービスがそれぞれバッチ処理を持つよりも、不可も分散できそう。
// もしmoduleやqueryに直接コマンドを追加したいという声が多ければ、後で追加すればいい。
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// helloworldCmd represents the helloworld command
var helloworldCmd = &cobra.Command{
	Use:   "helloworld",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("helloworld called")
	},
}

func init() {
	rootCmd.AddCommand(helloworldCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// helloworldCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// helloworldCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
