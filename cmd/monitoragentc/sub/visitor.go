package sub

// var visitorTypes = []v1.VisitorType{
// 	v1.VisitorTypeSTCP,
// 	v1.VisitorTypeSUDP,
// 	v1.VisitorTypeXTCP,
// }

// func init() {
// 	cmd := NewVisitorCommand()
// 	clientCfg := v1.ClientCommonConfig{}
// 	config.RegisterClientCommonConfigFlags(cmd, &clientCfg)
// 	for _, typ := range visitorTypes {
// 		if lo.Contains(visitorTypes, v1.VisitorType(typ)) {
// 			vc := v1.NewVisitorConfigurerByType(v1.VisitorType(typ))
// 			if vc == nil {
// 				panic("visitor type: " + typ + " not support")
// 			}
// 			subCommand := NewVisitorSubCommand(string(typ), vc, &clientCfg)
// 			config.RegisterVisitorFlags(subCommand, vc)
// 			cmd.AddCommand(subCommand)
// 		}
// 	}
// 	rootCmd.AddCommand(cmd)
// }

// func NewVisitorCommand() *cobra.Command {
// 	return &cobra.Command{
// 		Use:   "visitor",
// 		Short: "Run monitoragentc with visitor",
// 		RunE: func(cmd *cobra.Command, args []string) error {
// 			return nil
// 		},
// 	}
// }

// func NewVisitorSubCommand(typ string, c v1.VisitorConfigurer, clientCfg *v1.ClientCommonConfig) *cobra.Command {
// 	return &cobra.Command{
// 		Use:   typ,
// 		Short: fmt.Sprintf("Run monitoragentc with a single %s visitor", typ),
// 		Run: func(cmd *cobra.Command, args []string) {
// 			clientCfg.Complete()
// 			if _, err := validation.ValidateClientCommonConfig(clientCfg); err != nil {
// 				fmt.Println(err)
// 				os.Exit(1)
// 			}

// 			c.Complete(clientCfg)
// 			c.GetBaseConfig().Type = typ
// 			if err := validation.ValidateVisitorConfigurer(c); err != nil {
// 				fmt.Println(err)
// 				os.Exit(1)
// 			}
// 			err := startVisitorService(clientCfg, []v1.VisitorConfigurer{c})
// 			if err != nil {
// 				fmt.Println(err)
// 				os.Exit(1)
// 			}
// 		},
// 	}
// }

// func startVisitorService(
// 	cfg *v1.ClientCommonConfig,
// 	visitorCfgs []v1.VisitorConfigurer,
// ) error {
// 	log.InitLog(cfg.Log.To, cfg.Log.Level, cfg.Log.MaxDays, cfg.Log.DisablePrintColor)

// 	svc, err := client.NewService(client.ServiceOptions{
// 		Common:      cfg,
// 		VisitorCfgs: visitorCfgs,
// 	})
// 	if err != nil {
// 		return err
// 	}

// 	shouldGracefulClose := cfg.Transport.Protocol == "kcp" || cfg.Transport.Protocol == "quic"
// 	// Capture the exit signal if we use kcp or quic.
// 	if shouldGracefulClose {
// 		go handleTermSignal(svc)
// 	}

// 	return svc.Run(context.Background())
// }
