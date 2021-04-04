SHELL := /bin/bash
SCRIPTS_DIR := ./scripts

deploy:
	@$(SHELL) ./deploy.sh all $(FLAGS)

deploy-ais:
	@$(SHELL) ./deploy.sh ais $(FLAGS)

destroy:
	@$(SHELL) ./destroy.sh all $(FLAGS)

destroy-ais:
	@$(SHELL) ./destroy.sh ais $(FLAGS)

ci-prepare:
	@$(SHELL) $(SCRIPTS_DIR)/ci-prepare.sh

ci-deploy:
	@$(SHELL) $(SCRIPTS_DIR)/ci-deploy.sh

ci-deploy-k8s:
	@$(SHELL) $(SCRIPTS_DIR)/ci-deploy-k8s.sh $(ARGS)

ci-test:
	@$(SHELL) $(SCRIPTS_DIR)/ci-test-run.sh
