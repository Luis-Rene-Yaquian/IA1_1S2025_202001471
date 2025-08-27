% ======= MediLogic KB (auto-generado) =======
% NO editar a mano; use /admin/kb

sintoma(fiebre).
sintoma(tos).
sintoma(dolor_garganta).
sintoma(disnea).
sintoma(dolor_pecho).
sintoma(cefalea).
sintoma(nausea).

enfermedad(gripe, "Gripe", respiratorio, viral).

enf_sintoma(gripe, fiebre).
enf_sintoma(gripe, tos).
enf_sintoma(gripe, dolor_garganta).

% --- TODOS los medicamento/1 juntos
medicamento(paracetamol).
medicamento(ibuprofeno).

% --- TODOS los trata/2 juntos
trata(paracetamol, gripe).
trata(ibuprofeno, gripe).

% --- TODOS los contraindicado/2 juntos
contraindicado(paracetamol, alergia_paracetamol).
contraindicado(ibuprofeno, ulcera_gastrica).
