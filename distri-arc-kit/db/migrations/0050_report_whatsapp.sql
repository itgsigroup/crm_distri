-- +goose Up
-- Report WhatsApp (ADR 0050): a page of its own; every role that reads Chat gets it (a sales sees only their own
-- conversations there, as in Chat). Roles without a screen list (null = every screen of the base) need nothing.
update roles set screens = array_append(screens, 'reportwa')
where screens is not null and 'chat' = any(screens) and not ('reportwa' = any(screens));

-- +goose Down
update roles set screens = array_remove(screens, 'reportwa') where screens is not null;
