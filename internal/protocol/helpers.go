package protocol

import "strings"

// SplitAndTrim 按分隔符切分字符串并去除各项首尾空格、过滤空项
func SplitAndTrim(s, sep string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	var res []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

// AttachClashTransport 为 Clash proxy 注入传输层配置 (ws, grpc, xhttp, httpupgrade)
func AttachClashTransport(proxy map[string]interface{}, network string, node *NodeConfig) {
	if node == nil || proxy == nil {
		return
	}
	switch network {
	case "ws":
		wsOpts := map[string]interface{}{}
		if p := node.GetParam("path"); p != "" {
			wsOpts["path"] = p
		}
		if h := node.GetParam("host"); h != "" {
			wsOpts["headers"] = map[string]string{"Host": h}
		}
		if len(wsOpts) > 0 {
			proxy["ws-opts"] = wsOpts
		}
	case "grpc":
		grpcOpts := map[string]interface{}{}
		if sn := node.GetParam("serviceName"); sn != "" {
			grpcOpts["grpc-service-name"] = sn
		}
		if len(grpcOpts) > 0 {
			proxy["grpc-opts"] = grpcOpts
		}
	case "xhttp", "splithttp":
		xhttpOpts := map[string]interface{}{}
		if p := node.GetParam("path"); p != "" {
			xhttpOpts["path"] = p
		}
		if h := node.GetParam("host"); h != "" {
			xhttpOpts["headers"] = map[string]string{"Host": h}
		}
		if m := node.GetParam("mode"); m != "" {
			xhttpOpts["mode"] = m
		}
		if len(xhttpOpts) > 0 {
			proxy["xhttp-opts"] = xhttpOpts
		}
	case "httpupgrade":
		huOpts := map[string]interface{}{}
		if p := node.GetParam("path"); p != "" {
			huOpts["path"] = p
		}
		if h := node.GetParam("host"); h != "" {
			huOpts["headers"] = map[string]string{"Host": h}
		}
		if len(huOpts) > 0 {
			proxy["httpupgrade-opts"] = huOpts
		}
	}
}

// AttachSingBoxTransport 为 Sing-box outbound 注入传输层配置 (ws, grpc, xhttp, httpupgrade)
func AttachSingBoxTransport(outbound map[string]interface{}, network string, node *NodeConfig) {
	if node == nil || outbound == nil {
		return
	}
	switch network {
	case "ws":
		transport := map[string]interface{}{
			"type": "ws",
		}
		if p := node.GetParam("path"); p != "" {
			transport["path"] = p
		}
		if h := node.GetParam("host"); h != "" {
			transport["headers"] = map[string]string{"Host": h}
		}
		outbound["transport"] = transport
	case "grpc":
		transport := map[string]interface{}{
			"type": "grpc",
		}
		if sn := node.GetParam("serviceName"); sn != "" {
			transport["service_name"] = sn
		}
		outbound["transport"] = transport
	case "xhttp", "splithttp":
		transport := map[string]interface{}{
			"type": "xhttp",
		}
		if p := node.GetParam("path"); p != "" {
			transport["path"] = p
		}
		if h := node.GetParam("host"); h != "" {
			transport["headers"] = map[string]string{"Host": h}
		}
		if m := node.GetParam("mode"); m != "" {
			transport["mode"] = m
		}
		outbound["transport"] = transport
	case "httpupgrade":
		transport := map[string]interface{}{
			"type": "httpupgrade",
		}
		if p := node.GetParam("path"); p != "" {
			transport["path"] = p
		}
		if h := node.GetParam("host"); h != "" {
			transport["headers"] = map[string]string{"Host": h}
		}
		outbound["transport"] = transport
	}
}
