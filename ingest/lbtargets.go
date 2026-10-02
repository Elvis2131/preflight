// This file is PC-150: load balancer target registration read from HCL, as the same
// load-balancer -> target routes_to edge (no destination_cidr) the canvas authors, so the
// target_deregistration fault (PC-130) and the trace's target_registration step work on
// Terraform designs too.
//
// Chain: aws_lb_listener (default_action forward -> target group, or a listener rule's forward
// action) -> aws_lb_target_group <- aws_lb_target_group_attachment (target_id). Per the ELB User
// Guide, "Target groups for your Application Load Balancers": "You can use each target group with
// only one load balancer", so a target group names its load balancer unambiguously, and the target
// type "determines the type of target you specify when registering targets": an instance ID, an
// IP address, or a Lambda function. The Terraform aws_lb_target_group_attachment docs say target_id
// is "the Instance ID for an instance ... If the target type is lambda, specify the Lambda function
// ARN. If the target type is alb, specify the ALB ARN."
//
// WHAT THIS CAN AND CANNOT SAY. An attachment resource is a positive fact: that target is
// registered. Absence of an attachment is NOT a fact: the same guide says "After you attach a
// target group to an Auto Scaling group, Auto Scaling registers your targets with the target group
// for you", and ECS services and the Kubernetes load balancer controller register targets too,
// none of which an attachment resource shows. So every edge built here is stamped
// registration_scope = "attachments": core may conclude "registered", never "not registered".
// Targets of type "ip" are skipped: an address does not identify an IR node (no per-resource IP is
// modelled), so guessing which node it was would be exactly the invented fact I4 forbids.
package ingest

import (
	"fmt"
	"sort"

	"preflight/core"
	"preflight/providers"
)

// RegistrationScopeAttachments marks an edge derived only from attachment resources.
const RegistrationScopeAttachments = "attachments"

// targetTypeResource maps a target group's literal target_type to the resource type whose node an
// attachment's target_id may reference. An absent target_type is "instance" (the provider default).
var targetTypeResource = map[string]string{
	"instance": "aws_instance",
	"lambda":   "aws_lambda_function",
	"alb":      "aws_lb",
}

func buildLBTargetEdges(parsed []ParsedResource, byKey map[string]ParsedResource, registry providers.Registry) []core.Edge {
	// listener key -> load balancer
	listenerLB := map[string]ResourceRef{}
	for _, r := range parsed {
		if r.Type != "aws_lb_listener" {
			continue
		}
		if refs := r.AttributeReferences["load_balancer_arn"]; len(refs) == 1 && refs[0].ResourceType == "aws_lb" {
			listenerLB[r.Key()] = refs[0]
		}
	}

	// target group key -> load balancers that forward to it
	tgLBs := map[string]map[string]ResourceRef{}
	addForward := func(lb ResourceRef, nb NestedBlock) {
		if nb.Attributes["type"] != "forward" {
			return
		}
		tg, ok := nb.References["target_group_arn"]
		if !ok || tg.ResourceType != "aws_lb_target_group" {
			return
		}
		if tgLBs[tg.Key()] == nil {
			tgLBs[tg.Key()] = map[string]ResourceRef{}
		}
		tgLBs[tg.Key()][lb.Key()] = lb
	}
	for _, r := range parsed {
		switch r.Type {
		case "aws_lb_listener":
			if lb, ok := listenerLB[r.Key()]; ok {
				for _, nb := range r.NestedBlocks["default_action"] {
					addForward(lb, nb)
				}
			}
		case "aws_lb_listener_rule":
			refs := r.AttributeReferences["listener_arn"]
			if len(refs) != 1 {
				continue
			}
			if lb, ok := listenerLB[refs[0].Key()]; ok {
				for _, nb := range r.NestedBlocks["action"] {
					addForward(lb, nb)
				}
			}
		}
	}

	seen := map[string]bool{}
	var edges []core.Edge
	for _, r := range parsed {
		if r.Type != "aws_lb_target_group_attachment" {
			continue
		}
		tgRefs := r.AttributeReferences["target_group_arn"]
		idRefs := r.AttributeReferences["target_id"]
		if len(tgRefs) != 1 || len(idRefs) != 1 {
			continue
		}
		tgKey, targetRef := tgRefs[0].Key(), idRefs[0]
		tg, ok := byKey[tgKey]
		if !ok {
			continue
		}
		// The target must be the kind of resource this target group's type registers.
		targetType := "instance"
		if v, declared := tg.Attributes["target_type"]; declared {
			s, isString := v.(string)
			if !isString {
				continue
			}
			targetType = s
		} else if tg.NonLiteralAttributes["target_type"] {
			continue
		}
		if want, known := targetTypeResource[targetType]; !known || targetRef.ResourceType != want {
			continue
		}
		if !nodeWillExist(targetRef, byKey, registry) {
			continue
		}
		for _, lb := range tgLBs[tgKey] {
			if !nodeWillExist(lb, byKey, registry) {
				continue
			}
			lbKey := lb.Key()
			id := fmt.Sprintf("%s-target[%s]->%s", lbKey, tgKey, targetRef.Key())
			if seen[id] {
				continue
			}
			seen[id] = true
			edges = append(edges, core.Edge{
				ID:         id,
				Type:       core.EdgeTypeRoutesTo,
				From:       lbKey,
				To:         targetRef.Key(),
				Resolution: core.ResolutionKnown,
				Provenance: core.NewProvenance(core.KindStated, sourceRef(r)),
				RawAttributes: map[string]any{
					"target_group":       tgKey,
					"registration_scope": RegistrationScopeAttachments,
				},
			})
		}
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	return edges
}
