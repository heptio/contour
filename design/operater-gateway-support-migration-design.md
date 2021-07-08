# Contour Operator Gateway API Support Migration

Status: Draft

## Abstract
Currently the Contour Operator and Contour Controller have both started implementing Gateway API support.
Both components contain controllers that reconcile GatewayClass and Gateway resources and that conflict when reconciling status.
This document describes options for how we should proceed to untangle these.

## Background
Currently the state of the world is as below:

### Contour Operator
- Configured with a GatewayClass object reference (name) to reconcile
- Includes GatewayClass controller
  - Checks if named GatewayClass has the correct (hardcoded to `projectcontour.io/contour-operator`) controller string
  - Checks for Contour CRD in the parameters ref of the GatewayClass, uses parameters for provisioning Contour instances
  - Sets admitted status on object if all valid
- Includes Gateway controller
  - Checks if Gateway refers to correct GatewayClass and that GatewayClass has been admitted
  - Sets up finalizer for Gateway
  - Creates Contour deployment resources with the Gateway being reconciled named in Contour's ConfigMap
  - Sets up finalizer for GatewayClass
  - Sets up finalizer for Contour CRD referenced in GatewayClass
  - Sets admitted status on object if all valid

### Contour Controller
- Needs to be configured with a GatewayClass controller name to ensure it reconciles the correct GatewayClass
- Also needs a Gateway namespace/name which is being deprecated
- Includes GatewayClass controller
  - Expects parameter ref field to be unset, otherwise GatewayClass is not admitted
  - Admits GatewayClass that has the configured controller name
- Includes Gateway controller
  - Checks if Gateway references Contour owned GatewayClass
  - Passes off reconciliation to dag package Gateway API processor
  - Gateway is validated, status set on it and relevant *Routes

### Current Problems
- Contour requires Gateway namespace/name *and* controller name, the latter of which the operator does not provide
- Contour expects GatewayClass parameters ref to be unset while Operator expects it to reference a Contour CRD
- If the above is rectified, Contour and the Operator still will conflict on setting status
- CI failures that occur because of conflicts between Contour and the Operator are not detected unless we manually look at the scheduled Operator CI job

## Goals
- Improve implementation in Contour and Operator to mitigate current issues
- Improve development processes and practices to ensure this conflicting state does not occur again

## Non Goals
- Drop support for features or projects

## Remediation Steps

- Set up alerting on Operator CI failures
  - So we catch issues sooner.
- Operator sets controller name in Contour ConfigMap
  - To ensure Contour does not immediately exit on startup.
- Operator stops setting any status on Gateway API resources
  - To ensure Contour and Operator do not "fight" over status
- Contour does not reject GatewayClasses if parameters ref is set
  - At this point, we should have a workable intermediate solution for the moment and can cut a patch release
  - Reconciliation will be duplicated but this should "work" for now
  - User creates GatewayClass/Contour CRD with GatewayClass name
  - Operator reconciles, does not set status
  - User creates Gateway referencing GatewayClass
  - Operator reconciles, creates a Contour
  - Contour reconciles GatewayClass and Gateway, sets status, etc.
- Operator deprecates Contour CRD GatewayClassRef field
  - Currently the gateway reference has to be a GatewayClass name which is then validated to have the correct (hardcoded) controller string
  - We should instead ensure the CRD has a field for setting the controller string the instance of Contour will watch for and reconcile GatewayClasses for
  - The Operator should also set status to invalid on Contour objects that try to use a GatewayClass controller string that already exists, as it doesn't really make sense for multiple instances of Contour to watch for the same Controller string
- Contour no longer requires Gateway namespace/name to be configured
  - This is a planned deprecation anyway
  - Will help with next step
- Remove GatewayClass and Gateway controllers from Operator
  - Planned deprecation anyways
  - Users will no longer create a GatewayClass that references a Contour CRD to get a Contour that is configured for Gateway API.
  - Instead, users will create a Contour CRD with a GatewayClass name and the operator will turn that into a Contour with the appropriate config.
  - Operator will need to expand on Contour CRD controller, ensure Contour deployment resources are created when GatewayClass ref is set on Contour object
- Move Operator into main Contour repository (long-term/optional)
  - This option is a longer term project that would help us sort out process issues between Contour and Operator development.
  - We would move the Operator codebase into the Contour one and deprecate/remove the Operator specific repo.
  - Pros
    - Allows us to test, get feedback in one repository, each PR will be tested against Contour, Contour integration in Operator
    - Operator can be tested against specific Contour image versions, currently Operator in CI is just pointed at the `main` tag so we don't always know what is tested in a CI run
    - Shared code for common functions, no need to extract libraries and sync versions across repos.
  - Cons
    - Doesn't follow typical operator/component repo split pattern
    - Changes unrelated to operator will cause CI to run, expensive+time consuming
